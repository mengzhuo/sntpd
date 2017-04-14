package sntpd

import "testing"

func TestCheckPosition(t *testing.T) {
	checkList := map[int]int{
		LiVnMode:           0,
		Stratum:            1,
		Poll:               2,
		ClockPrecision:     3,
		RootDelayPos:       4,
		RootDispersionPos:  8,
		ReferIDPos:         12,
		ReferenceTimeStamp: 16,
		OriginTimeStamp:    24,
		ReceiveTimeStamp:   32,
		TransmitTimeStamp:  40,
	}
	for k, v := range checkList {
		if k != v {
			t.Errorf("position check error expect:%d, get:%d", k, v)
		}
	}
}
