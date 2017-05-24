package sntpd

import (
	"log"
	"math"
	"sort"
	"time"
)

const (
	MinStep     = 300 * time.Second
	MaxChange   = 128 * time.Millisecond
	AllanXpt    = time.Hour
	MaxDistance = 1.5 // second
	NStage      = 8
	MaxStratum  = 16
)

const (
	TypeLower = -1 + iota
	TypeMid
	TypeUpper
)

type Clock struct {
	peer      []*Peer
	epoch     time.Time
	freqCount int
	poll      int8
	precision int8
}

func pollToDuration(poll int8) time.Duration {
	return time.Duration(math.Pow(2, float64(poll)) * float64(time.Second))
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

func (s *Service) monitor() {
	// peer nexttime will be update by Select or Update
	ticker := time.NewTicker(10 * time.Second)
	now := time.Now()
	var (
		offset, disp float64
		delay        float64
		err          error
		changed      bool
	)

	for {
		<-ticker.C
		now = time.Now()
		for _, p := range s.clock.peer {
			if p.nextTime.After(now) {
				continue
			}
			changed = true
			p.nextTime = now.Add(pollToDuration(s.clock.poll))
			offset, disp, delay, err = p.query()
			if err != nil && p.reach&0xfc == 0 {
				s.clockFilter(p, 0, MaxDispersion.Seconds(), MaxDispersion.Seconds())
				continue
			}
			s.clockFilter(p, offset, disp, delay)
		}
		if !changed {
			continue
		}

		s.clockSelect()
		s.clockUpdate()
		changed = false
	}
}

func (s *Service) clockFilter(p *Peer, offset, disp, delay float64) {
	now := time.Now()
	dTemp := Phi * now.Sub(p.epoch).Seconds()
	p.epoch = now
	// shift right
	for i := NStage - 1; i > 0; i-- {
		p.filter[i] = p.filter[i-1]
	}
	p.filter[0].offset = offset
	p.filter[0].disp = disp
	p.filter[0].delay = delay

	for i := 0; i < NStage; i++ {
		if i != 0 {
			p.filter[i].disp += dTemp
		}

		p.filterDistance[i].index = i
		switch {
		case p.filter[i].disp > MaxDispersion.Seconds():
			p.filter[i].disp = MaxDispersion.Seconds()
			p.filterDistance[i].distance = MaxDispersion.Seconds()
		case now.Sub(p.filter[i].epoch) > AllanXpt:
			p.filterDistance[i].distance = p.filter[i].delay + p.filter[i].disp
		default:
			p.filterDistance[i].distance = p.filter[i].delay
		}
	}

	if s.clock.freqCount == 0 {
		sort.Sort(byDistance(p.filterDistance))
	}

	// find match filter
	m := 0
	for _, fd := range p.filterDistance {
		if fd.distance >= MaxDispersion.Seconds() || (m > 2 && fd.distance >= MaxDistance) {
			continue
		}
		m += 1
	}

	p.disp = 0
	p.jitter = 0

	bestF := p.filter[p.filterDistance[0].index]

	var j int
	for i := NStage - 1; i > 0; i-- {
		j = p.filterDistance[i].index
		p.disp = (p.disp + p.filter[j].delay) / 2

		if i < m {
			p.jitter += math.Pow(p.filter[j].delay-bestF.delay, 2)
		}
	}

	if m == 0 {
		return
	}

	//etemp := fabs(p.offset.Seconds() - bestF.Offset.Seconds())

	p.offset = bestF.offset
	p.delay = bestF.delay
	if m > 1 {
		p.jitter /= float64(m - 1)
	}
	p.jitter = float64Max(math.Pow(p.jitter, 2), log2D(s.clock.precision))

	if bestF.epoch.After(p.epoch) {
		log.Printf("clockFilter: old sample %s", bestF.epoch)
		return
	}

	p.epoch = bestF.epoch
}

func float64Max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

type Interset struct {
	peer *Peer
	val  float64
	typE int
}

func (s *Service) fit(p *Peer) (err error) {
	if p.leap == NotSync {
		return PeerNotSync
	}
	if p.stratum >= MaxStratum {
		return PeerInvalidStratum
	}

	if p.rootDistance > MaxDistance+Phi*log2D(s.clock.poll) {
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

func (s *Service) clockSelect() {

	samples := []Interset{}
	for _, p := range s.clock.peer {
		if err := s.fit(p); err != nil {
			log.Printf("clockSelect: %s not fit, reason:%s", p.addr, err)
			continue
		}
		samples = append(samples, Interset{p, p.offset - p.rootDistance, TypeLower})
		samples = append(samples, Interset{p, p.offset + p.rootDistance, TypeUpper})
	}

	if len(samples) < 2 {
		log.Printf("clockSelect: not enough peer to select cluster surviors")
		return
	}
	s.marzullo(samples)
}

func (s *Service) marzullo(iset []Interset) (surviors []*Peer) {

	sort.Sort(byOffset(iset))
	nlist := len(iset) / 2
	nl2 := len(iset)

	var n int
	low := float64(3600)
	high := float64(-3600)

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

func (s *Service) clockUpdate() {

}
