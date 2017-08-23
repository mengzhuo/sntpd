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
	sysIndex  int
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
	var (
		now     time.Time
		changed bool
	)

	for {
		now = time.Now()
		for _, p := range s.clock.peer {
			if p.nextTime.After(now) {
				continue
			}
			p.nextTime = now.Add(pollToDuration(s.clock.poll))
			p.query()
			if p.reach&1 == 0 {
				continue
			}
			changed = true
		}
		time.Sleep(time.Second)

		if !changed {
			continue
		}

		s.clockSelect()
		s.clockUpdate()
		changed = false
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
	for _, p := range s.clock.peer {
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
