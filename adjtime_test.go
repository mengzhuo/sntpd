package sntpd

import "testing"

func TestSystemClockSyncing(t *testing.T) {
	t.Log(systemClockSyncing())
}
