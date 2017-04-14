package sntpd

import "time"

const (
	maxStratum = 16
	nanoPerSec = 1000000000
)

var (
	ntpEpoch = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
)

type NTPMessage struct {
	LiVnMode  byte
	Stratum   uint8
	Poll      int8
	Precision int8 // 4

	RooTDelay      uint32 // 4
	RooTDespersion uint32
	ReferenceID    uint32 // 4*4 = 16

	RefTimeStamp      uint64 // 8
	OriginTimeStamp   uint64
	ReceiveTimeStamp  uint64
	TransmitTimeStamp uint64
}

func Duration(t uint64) time.Duration {
	sec := (t >> 16) * nanoPerSec
	frac := (t & 0xffff) * nanoPerSec >> 16
	return time.Duration(sec + frac)
}
