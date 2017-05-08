package sntpd

import (
	"fmt"
	"log"
	"syscall"
	"time"
)

const (
	MAX_ADJUST    = int64(400 * time.Millisecond)
	NanoSecPerSec = int64(time.Second)
)

func i64abs(a int64) int64 {
	if a < 0 {
		return -a
	}
	return a
}

func (s *Service) setOffsetToSystem(offset time.Duration, leap uint8) (driftPPM float64, jumped bool, err error) {

	tmx := &syscall.Timex{}
	offsetNsec := offset.Nanoseconds()

	if i64abs(offsetNsec) < MAX_ADJUST {
		tmx.Modes = ADJ_STATUS | ADJ_NANO | ADJ_OFFSET | ADJ_TIMECONST | ADJ_MAXERROR | ADJ_ESTERROR
		tmx.Status = STA_PLL
		tmx.Offset = offsetNsec
		tmx.Constant = int64(s.poll - 4)
		tmx.Maxerror = 0
		tmx.Esterror = 0
	} else {
		tmx.Modes = ADJ_STATUS | ADJ_NANO | ADJ_SETOFFSET
		tmx.Time.Sec = offsetNsec / NanoSecPerSec
		tmx.Time.Usec = offsetNsec - (tmx.Time.Sec * NanoSecPerSec)

		if tmx.Time.Usec < 0 {
			tmx.Time.Sec -= 1
			tmx.Time.Usec += NanoSecPerSec
		}
		jumped = true
	}

	switch leap {
	case LeapIns:
		tmx.Status |= STA_INS
	case LeapDel:
		tmx.Status |= STA_DEL
	}

	var rc int
	rc, err = syscall.Adjtimex(tmx)
	if rc != 0 {
		return 0, true, fmt.Errorf("rc=%d status=%s", rc, statusToString(tmx.Status))
	}
	driftPPM = float64(tmx.Freq) / 65536
	return
}

func absInt64(a int64) int64 {
	if a < 0 {
		return -a
	}
	return a
}

func initClock(freq int64) {
	log.Print("init clock")
	tmx := &syscall.Timex{}
	tmx.Modes = ADJ_TICK | ADJ_FREQUENCY | ADJ_SETOFFSET
	tmx.Tick = 10000
	tmx.Freq = freq
	tmx.Offset = 0
	r, err := syscall.Adjtimex(tmx)
	if err != nil || r != 0 {
		log.Print("initClock err: ", err, "status=%s", statusToString(tmx.Status))
	}
}
