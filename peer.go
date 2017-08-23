package sntpd

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/beevik/ntp"
)

const (
	Phi           float64 = 15e-6 // A.K.A frequency torlarence 15e-6 s / s
	NotSync               = 0x03
	MaxDispersion         = 16 * time.Second
	MaxStratum            = 16
)

const (
	PeerInit Status = iota
	PeerUnavailable
	PeerRejected
	PeerFalseTick
	PeerOutLyer
	PeerCandidate
	PeerSys
)

func log2D(x int8) float64 {
	return math.Ldexp(1, int(x))
}

func durationToPoll(t time.Duration) int8 {
	return int8(math.Log2(float64(t)))
}

type Status uint8

type Peer struct {
	addr  string
	index int

	epoch    time.Time
	nextTime time.Time

	offset float64
	jitter float64
	disp   float64
	delay  float64

	rootDistance   float64
	rootDelay      float64
	rootDispersion float64

	refid   uint32
	ppoll   int8
	reach   uint8
	leap    uint8
	stratum uint8
	status  Status
}

func (p *Peer) query() {
	p.reach <<= 1
	resp, err := ntp.Query(p.addr, 4)
	if err != nil {
		log.Print(err, p.addr)
		return
	}

	if resp.Stratum == 0 || resp.Stratum > MaxStratum {
		log.Print("InvalidStratum", p.addr)
		return
	}

	if resp.Leap&NotSync == NotSync {
		log.Print("NotSync", p.addr)
		return
	}

	p.rootDistance = (resp.RootDelay + resp.RootDispersion/2).Seconds()

	if p.rootDistance > MaxDispersion {
		log.Print("RootDistanceTooBig", p.addr)
		return
	}
	p.reach |= 1

	p.rootDispersion = resp.RootDispersion.Seconds()
	p.rootDelay = resp.RootDelay.Seconds()

	p.stratum = resp.Stratum
	p.leap = resp.Leap
	p.refid = resp.ReferenceID
	p.ppoll = durationToPoll(resp.Poll)
	p.disp = resp.Precision.Seconds() + Phi*resp.RTT.Seconds()
	p.offset = resp.CloseOffset.Seconds()
	p.delay = resp.RTT.Seconds()

	return
}

func (s *Service) monitorPeer(p *Peer) {
	interval := time.Second
	for {
		time.Sleep(interval)
		p.query()
		interval := pollToDuration(p.ppoll)
		if p.reach&1 == 0 {
			// not reached
		}
	}
}

func (p *Peer) String() string {
	return fmt.Sprintf("%s offset:%f delay:%f disp:%f reach:%d", p.addr, p.offset, p.delay, p.disp, p.reach)
}
