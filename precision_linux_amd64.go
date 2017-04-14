package sntpd

import (
	"math"
	"syscall"
)

func systemPrecision() int8 {
	tmx := &syscall.Timex{}
	syscall.Adjtimex(tmx)
	// linux 1 for usec
	return int8(math.Log2(float64(tmx.Precision) * 1e-6))
}
