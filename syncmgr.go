package sntpd

import (
	"errors"
	"log"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/beevik/ntp"
)

/*

Simpilfied ntp sync algorthim

* treat all peers as upper level stratum

https://tools.ietf.org/html/rfc5905#section-4
Definition

The offset (theta) represents the
maximum-likelihood time offset of the server clock relative to the
system clock.  The delay (delta) represents the round-trip delay
between the client and server.  The dispersion (epsilon) represents
the maximum error inherent in the measurement.  It increases at a
rate equal to the maximum disciplined system clock frequency
tolerance (PHI), typically 15 ppm.  The jitter (psi) is defined as
the root-mean-square (RMS) average of the most recent offset
differences, and it represents the nominal error in estimating the
offset.

Note:
The original algorthim use second with float64 (double) for these four
units, however Go use int64 represents time.Duration at percision nanosecond,
we use this Go's time.Duration through out the project for convience.
*/

const (
	MaxDispersion = 16 * time.Second
	MinDispersion = 5 * time.Millisecond
	HighMark      = -time.Hour
	LowMark       = time.Hour

	MaxDistance = 1500 * time.Millisecond
	SGate       = 3
	Allan       = 11
	AllanXpt    = (1 << Allan) * time.Second

	SampleMaxCount       = 32
	MaxStratum     uint8 = 16
	NStage               = 8

	Phi float64 = 15e-6 // A.K.A frequency torlarence 15e-6 s / s
)

const (
	NoLeap uint8 = iota
	LeapIns
	LeapDel
	NotSync
)

const (
	TypeLower = -1 + iota
	TypeMid
	TypeUpper
)

var (
	PeerNotAvailable       = errors.New("peer not available")
	PeerNotSync            = errors.New("not sync")
	PeerInvalidStratum     = errors.New("invalid stratum")
	PeerRootDistanceTooBig = errors.New("root distance too big")
	PeerOldTimer           = errors.New("peer is faster than current time")
)

func SquareOffset(x float64) float64 {
	return x * x
}

type ClockFilter struct {
	Offset, Delay, Disp time.Duration
	Epoch               time.Time
}

type Peer struct {
	Addr                string
	offset, delay, disp time.Duration
	rootDelay, rootDisp time.Duration
	rootDistance        time.Duration
	jitter              float64
	nextPoll            time.Duration
	epoch               time.Time
	Filter              []ClockFilter
	filterDistance      []filterDistance
	pollCounter         uint64
	refid               uint32
	poll                int8
	reach               uint8
	leap                uint8
	stratum             uint8
}

func (s *Service) queryPeer(p *Peer) (resp *ntp.Response, disp time.Duration, err error) {
	resp, err = ntp.Query(p.Addr, 4)
	p.reach <<= 1
	if err != nil {
		return
	}
	if resp.Stratum == 0 || resp.Stratum > 15 {
		return nil, MaxDispersion, PeerInvalidStratum
	}

	if resp.Leap == NotSync {
		return nil, MaxDispersion, PeerNotSync
	}

	if resp.RootDelay/2+resp.RootDispersion > MaxDispersion {
		return nil, MaxDispersion, PeerRootDistanceTooBig
	}

	p.rootDisp = resp.RootDispersion
	p.rootDelay = resp.RootDelay
	p.leap = resp.Leap
	p.stratum = resp.Stratum
	p.refid = resp.ReferenceID
	p.poll = durationToPoll(resp.Poll)
	p.reach |= 1

	disp = secondToDuration(log2D(s.precision)) + resp.Precision + secondToDuration(Phi*resp.RTT.Seconds())
	return
}

func log2D(x int8) float64 {
	return math.Ldexp(1, int(x))
}

func fabs(a float64) float64 {
	if a < 0 {
		return -a
	}
	return a
}

func (p *Peer) init() {
	p.Filter = make([]ClockFilter, NStage)
	for i := 1; i < NStage; i++ {
		p.Filter[i] = ClockFilter{
			Offset: 0 * time.Second,
			Delay:  MaxDispersion,
			Disp:   MaxDispersion,
			Epoch:  ntpEpoch,
		}
	}
	p.filterDistance = make([]filterDistance, NStage)
	p.disp = MaxDispersion
	p.delay = MaxDispersion
	p.epoch = time.Now().Truncate(10 * time.Second)
	p.poll = 4
}

type filterDistance struct {
	distance time.Duration
	index    int
}

type byDistance []filterDistance

func (b byDistance) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func (b byDistance) Less(i, j int) bool {
	return b[i].distance < b[j].distance
}

func (b byDistance) Len() int {
	return len(b)
}

// https://tools.ietf.org/html/rfc5905#appendix-A.5.2
func (s *Service) clockFilter(p *Peer, offset, delay, disp time.Duration, ct time.Time) {

	// Phi means ppm
	// So we calculate peer disp by adding more ppm during
	dTemp := secondToDuration(Phi * ct.Sub(p.epoch).Seconds())
	p.epoch = ct

	// shift right
	for i := NStage - 1; i > 0; i-- {
		p.Filter[i] = p.Filter[i-1]
	}

	p.Filter[0].Offset = offset
	p.Filter[0].Delay = delay
	p.Filter[0].Disp = disp
	p.Filter[0].Epoch = ct

	for i := 0; i < NStage; i++ {
		if i != 0 {
			p.Filter[i].Disp += dTemp
		}

		p.filterDistance[i].index = i
		switch {
		case p.Filter[i].Disp > MaxDispersion:
			p.Filter[i].Disp = MaxDispersion
			p.filterDistance[i].distance = MaxDispersion
		case ct.Sub(p.Filter[i].Epoch) > AllanXpt:
			p.filterDistance[i].distance = p.Filter[i].Delay + p.Filter[i].Disp
		default:
			p.filterDistance[i].distance = p.Filter[i].Delay
		}
	}

	// after clock is stablized
	if p.poll == s.cfg.MaxPoll {
		p.pollCounter += 1
	} else {
		p.pollCounter = 0
	}
	if p.pollCounter > NStage {
		sort.Sort(byDistance(p.filterDistance))
	}

	// find match filter
	m := 0
	for _, fd := range p.filterDistance {
		if fd.distance >= MaxDispersion || (m > 2 && fd.distance >= MaxDistance) {
			continue
		}
		m += 1
	}

	p.disp = 0
	p.jitter = 0

	bestF := p.Filter[p.filterDistance[0].index]

	var j int
	for i := NStage - 1; i > 0; i-- {
		j = p.filterDistance[i].index
		p.disp = (p.disp + p.Filter[j].Delay) / 2

		if i < m {
			p.jitter += Diff(p.Filter[j].Delay, bestF.Delay)
		}
	}

	if m == 0 {
		return
	}

	//etemp := fabs(p.offset.Seconds() - bestF.Offset.Seconds())

	p.offset = bestF.Offset
	p.delay = bestF.Delay
	if m > 1 {
		p.jitter /= float64(m - 1)
	}
	p.jitter = float64Max(math.Pow(p.jitter, 2), log2D(s.precision))

	if bestF.Epoch.After(p.epoch) {
		log.Printf("clockFilter: old sample %s", bestF.Epoch)
		return
	}

	p.epoch = bestF.Epoch
}

func float64Max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func logSample(pl []*Peer) {
	for _, p := range pl {
		log.Printf("Peer:%s offset=%s", p.Addr, p.offset)
	}

}

type Interset struct {
	peer *Peer
	val  time.Duration
	typE int
}

// https://tools.ietf.org/html/rfc5905#appendix-A.5.5.1
// The origin clock select algorithm (finding intersection) in RFC 5905 is too complex to
// implement and understand therefor we use Student's T distribution
// ( https://en.wikipedia.org/wiki/Student%27s_t-distribution )
// to make sure that we had an accure offset to "real" time
// along with sorted by jitter/disp/rootdistance
func (s *Service) clockSelect() (surviors []*Peer) {

	samples := []Interset{}
	for _, p := range s.peers {
		if err := s.fit(p); err != nil {
			log.Printf("clockSelect: %s not fit, reason:%s", p.Addr, err)
			continue
		}
		prd := rootDist(p)
		p.rootDistance = prd
		samples = append(samples, Interset{p, p.offset - prd, TypeLower})
		samples = append(samples, Interset{p, p.offset + prd, TypeUpper})
	}

	if len(samples) < 2 {
		log.Printf("clockSelect: not enough peer to select cluster surviors")
		return
	}
	return s.marzullo(samples)
}

func (s *Service) marzullo(iset []Interset) (surviors []*Peer) {

	sort.Sort(byOffset(iset))
	nlist := len(iset) / 2
	nl2 := len(iset)

	var n int
	low := time.Hour
	high := -time.Hour

	for allow := 0; 2*allow < nlist; allow++ {

		n = 0
		for _, set := range iset {
			low = set.val
			n -= set.typE
			if n >= nlist-allow {
				break
			}
		}

		n = 0
		for j := nl2 - 1; j > 0; j-- {
			high = iset[j].val
			n += iset[j].typE
			if n >= nlist-allow {
				break
			}
		}
		if high > low {
			break
		}
	}

	var p *Peer
	for i := 0; i < len(iset); i += 2 {
		p = iset[i].peer
		if high <= low || p.offset+p.rootDistance < low || p.offset-p.rootDistance > high {
			continue
		}
		surviors = append(surviors, p)
	}
	return

}

type byOffset []Interset

func (b byOffset) Less(i, j int) bool {
	return b[i].val < b[j].val
}

func (b byOffset) Len() int {
	return len(b)
}

func (b byOffset) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

type byDelay []*Peer

func (b byDelay) Less(i, j int) bool {
	return b[i].delay+b[i].disp/2 < b[j].delay+b[j].disp/2
}

func (b byDelay) Len() int {
	return len(b)
}

func (b byDelay) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func meanOffset(pl []*Peer) (t time.Duration) {
	for _, p := range pl {
		t += p.offset
	}
	return t / time.Duration(len(pl))
}

func stdDevOffset(mean time.Duration, pl []*Peer) (dev float64) {

	for _, p := range pl {
		sub := (p.offset - mean)
		dev += math.Pow(sub.Seconds(), 2)
	}
	return math.Sqrt(dev / float64(len(pl)))
}

func (s *Service) sample() {

	var wg sync.WaitGroup
	for _, peer := range s.peers {
		wg.Add(1)
		go func(p *Peer) {
			defer wg.Done()
			resp, disp, err := s.queryPeer(p)
			if err != nil {
				log.Printf("queryPeer %s: err=%s", p.Addr, err)
				return
			}
			s.clockFilter(p, resp.ClockOffset, resp.RTT, disp, time.Now())
		}(peer)
	}
	wg.Wait()
}

func (s *Service) peerPoll(p *Peer) {

	for {
		time.Sleep(pollToDuration(p.poll))
		resp, disp, err := s.queryPeer(p)
		if err == PeerNotAvailable {
			s.clockFilter(p, 0, 0, MaxDispersion, time.Now())
		}
		if err != nil {
			log.Printf("queryPeer %s: err=%s", p.Addr, err)
			continue
		}
		s.clockFilter(p, resp.ClockOffset, resp.RTT, disp, time.Now())
		s.clockReady <- struct{}{}
	}
}

func (s *Service) monitorPoll() {

	tick := time.NewTicker(4 * time.Second)
	var status uint8
	for {
		select {
		case <-tick.C:
			status |= 1
		case <-s.clockReady:
			status |= 2
		}

		if status != 3 {
			continue
		}
		status = 0

		surviors := s.clockSelect()
		if len(surviors) < 1 {
			log.Print("no one surved")
			return
		}

		p := surviors[0]
		s.clockCombine(surviors)

		var (
			jumped bool
			err    error
		)
		s.drift, jumped, err = s.setOffsetToSystem(s.offset, s.leap)
		if err != nil {
			log.Print("set system err", err)
		}
		s.setFromPeer(p)
		log.Printf("set system from:%s offset:%s, leap:%v drift:%v, jumped=%v",
			p.Addr, s.offset, s.leap, s.drift, jumped)
		log.Printf("set system from:%s root distance:%s, root delay:%s",
			p.Addr, rootDist(p), p.delay)
		return
	}
}

func Diff(a, b time.Duration) float64 {
	return math.Pow(a.Seconds()-b.Seconds(), 2)
}

func (s *Service) clockCombine(surviors []*Peer) {

	var x, y, z, w float64
	var leapCount [3]uint8

	for _, p := range surviors {
		x = 1 / rootDist(p).Seconds()
		y += x
		z += x * p.offset.Seconds()
		w += x * Diff(p.offset, s.offset)
		leapCount[p.leap] += 1 // we will panic if peer is NotSync
	}

	s.leap = getLeap(leapCount)
	s.offset = secondToDuration(z / y)
	s.jitter = secondToDuration(math.Sqrt(w/y + math.Pow(s.jitter.Seconds(), 2)))
}

func getLeap(cnt [3]uint8) uint8 {
	var maxS, maxI uint8
	for i, s := range cnt[:] {
		if s > maxS {
			maxI = uint8(i)
			maxS = s
		}
	}
	return maxI
}

func rootDist(p *Peer) time.Duration {
	ct := time.Now()
	return maxDuration(MinDispersion,
		(p.rootDelay+p.delay)/2+
			p.rootDisp+
			p.disp+
			secondToDuration(Phi*ct.Sub(p.epoch).Seconds())+
			secondToDuration(p.jitter))
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func secondToDuration(a float64) time.Duration {
	return time.Duration(a * float64(time.Second))
}

func (s *Service) fit(p *Peer) (err error) {

	if p.leap == NotSync {
		return PeerNotSync
	}
	if p.stratum >= MaxStratum {
		return PeerInvalidStratum
	}

	if rootDist(p) > MaxDistance+secondToDuration(Phi*log2D(s.poll)) {
		return PeerRootDistanceTooBig
	}
	if time.Now().Before(p.epoch) {
		return PeerRootDistanceTooBig
	}
	if p.reach&0x07 == 0 {
		return PeerNotAvailable
	}
	return
}
