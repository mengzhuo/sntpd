package sntpd

import (
	"errors"
	"math"
	"time"

	"github.com/beevik/ntp"
)

const (
	Phi float64 = 15e-6 // A.K.A frequency torlarence 15e-6 s / s

	MaxDispersion = 16 * time.Second
)

const (
	PeerRejected Status = 1 << iota
	PeerFalseTick
	PeerOutLyer
	PeerCandidate
	PeerSys
)

var (
	PeerNotAvailable       = errors.New("peer not available")
	PeerNotSync            = errors.New("not sync")
	PeerInvalidStratum     = errors.New("invalid stratum")
	PeerRootDistanceTooBig = errors.New("root distance too big")
	PeerOldTimer           = errors.New("peer is faster than current time")
)

func log2D(x int8) float64 {
	return math.Ldexp(1, int(x))
}

type Status uint8

type ClockFilter struct {
	offset, disp float64
	delay        float64
	epoch        time.Time
}

type Peer struct {
	addr string

	clockFilter []ClockFilter

	nextTime time.Time
	epoch    time.Time

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

func (p *Peer) query() (err error) {
	p.reach <<= 1
	resp, err := ntp.Query(p.Addr, 4)
	if err != nil {
		return err
	}

	if resp.Stratum == 0 || resp.Stratum > 15 {
		return PeerInvalidStratum
	}

	if resp.Leap == NotSync {
		return PeerNotSync
	}

	if resp.RootDelay/2+resp.RootDispersion > MaxDispersion {
		return PeerRootDistanceTooBig
	}

	p.rootDispersion = resp.RootDispersion.Seconds()
	p.rootDelay = resp.RootDelay.Seconds()

	p.stratum = resp.Stratum
	p.leap = resp.Leap
	p.refid = resp.ReferenceID
	// XXX
	p.ppoll = durationToPoll(resp.Poll)
	disp := resp.Precision.Seconds() + Phi*resp.RTT.Seconds()
	p.reach |= 1
}
