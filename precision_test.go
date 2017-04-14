package sntpd

import "testing"

func TestSystemPrecision(t *testing.T) {
	a := systemPrecision()
	t.Logf("precision: %d", a)
}
