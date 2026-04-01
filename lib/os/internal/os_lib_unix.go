//go:build linux || darwin || freebsd || openbsd || netbsd
// +build linux darwin freebsd openbsd netbsd

package internal

import (
	"time"
)

// osClockImpl returns CPU time in seconds using wall clock on Unix.
// For accurate CPU time, use sys/time.h and getrusage() in C.
func osClockImpl() float64 {
	return float64(time.Since(startTime).Seconds())
}

// startTime records when the process started
var startTime = time.Now()
