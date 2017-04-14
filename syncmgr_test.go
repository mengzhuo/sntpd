package sntpd

import (
	"fmt"
	"testing"
	"time"
)

func TestClockFilter(t *testing.T) {
	p := &Peer{}
	p.init()
	s := &Service{peers: []*Peer{p}}
	s.cfg = DefaultConfig()
	s.precision = -16
	t.Logf("%d", s.precision)

	ct := time.Now()
	for i := 0; i < 11; i++ {
		///ii := i * 10
		s.clockFilter(p, secondToDuration(offsetList[i]),
			time.Duration(i)*time.Millisecond, time.Duration(i)*time.Millisecond,
			ct.Add(1*time.Minute))
		t.Logf("offset=%s delay=%s disp=%s jitter=%v ", p.offset, p.delay, p.disp, p.jitter)
	}
	s.clockFilter(p, secondToDuration(offsetList[0]),
		10*time.Millisecond, 10*time.Millisecond, ct)
}

var offsetList = []float64{
	0.00270919,
	-0.05169505,
	-0.051243523,
	-0.051375935,
	0.019738009,
	0.017742771,
	-0.003949342,
	-0.005308624,
	0.003457935,
	0.009821857,
	-0.004579132,
	0.123,
}

func TestClockSelect(t *testing.T) {
	s := &Service{}
	s.precision = -16
	s.peers = []*Peer{}

	for _, off := range offsetList {
		p := &Peer{}
		p.init()
		p.Addr = fmt.Sprintf("%f", off)
		p.offset = secondToDuration(off)
		s.peers = append(s.peers, p)
	}

	sl := s.clockSelect()
	for i, p := range sl {
		t.Log(i, p.offset)
	}
}

func TestMeanOffset(t *testing.T) {

	samples := []*Peer{}
	for i := 0; i < 15; i++ {
		samples = append(samples, &Peer{delay: time.Microsecond})
	}
	t.Log(meanOffset(samples))
}

func TestStdDevOffset(t *testing.T) {

	samples := []*Peer{}
	for i := 0; i < 15; i++ {
		samples = append(samples, &Peer{delay: time.Duration(i) * time.Microsecond})
	}
	t.Log(stdDevOffset(meanOffset(samples), samples))
}
