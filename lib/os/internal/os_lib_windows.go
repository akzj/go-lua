//go:build windows
// +build windows

package internal

import (
	"time"
)

// osClockImpl returns CPU time in seconds using wall clock on Windows.
// For accurate CPU time, use GetProcessTimes() in Windows API.
func osClockImpl() float64 {
	return float64(time.Since(startTime).Seconds())
}

// startTime records when the process started
var startTime = time.Now()
