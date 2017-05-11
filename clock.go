package sntpd

import "time"

const (
	MinStep   = 300 * time.Second
	MaxChange = 128 * time.Millisecond
)

type Clock struct {
	peer  []*Peer
	epoch time.Time
}

func (s *Service) monitor() {
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
				s.clockFilter(p, 0, MaxDispersion.Second(), MaxDispersion.Second())
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

}

func (s *Service) clockSelect() {

}

func (s *Service) clockUpdate() {

}
