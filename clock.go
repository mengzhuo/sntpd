package sntpd

import (
	"log"
	"math"
	"sort"
	"time"
)

const (
	MaxDistance = 1.5 // second
	NStage      = 8
)

const (
	TypeLower = -1 + iota
	TypeMid
	TypeUpper
)

type Clock struct {
	peer      []*Peer
	epoch     time.Time
	sysIndex  int
	poll      int8
	precision int8
}

func NewClock(cfg *Config) (clk *Clock) {
	clk = &Clock{sysIndex: -1}
	for i, addr := range cfg.ClockList {
		clk.peer = append(clk.peer, NewPeer(addr, i))
	}
	return
}

func pollToDuration(poll int8) time.Duration {
	return time.Duration(math.Pow(2, float64(poll)) * float64(time.Second))
}

func (s *Service) monitor() {
	// peer nexttime will be update by Select or Update
	var (
		now       time.Time
		changed   bool
		sleepTime time.Duration
	)

	for {
		time.Sleep(sleepTime)
		changed = false
		now = time.Now()
		for _, p := range s.clock.peer {
			if p.nextTime.After(now) {
				continue
			}
			p.query()
			p.nextTime = now.Add(pollToDuration(s.clock.poll))
			if p.reach&1 == 0 {
				continue
			}
			// some peer is changed
			changed = true
		}

		if !changed {
			continue
		}

		s.clockSelect()
	}
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

func (s *Service) clockSelect() {

	samples := []Interset{}
	for _, p = range s.clock.peer {
		if p.status < PeerFalseTick {
			continue
		}
		samples = append(samples, Interset{p, p.offset - p.rootDistance, TypeLower})
		samples = append(samples, Interset{p, p.offset + p.rootDistance, TypeUpper})
	}

	if len(samples) < 2 {
		log.Printf("clockSelect: not enough peer to select cluster surviors")
		return
	}
	surviors := s.marzullo(samples)

	for _, p := range surviors {
		if p.index == s.clock.index {
			log.Print("nothing changed")
			return
		}
	}

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
