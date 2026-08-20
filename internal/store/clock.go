package store

import "time"

// SystemClock reports the real wall-clock time. It is the clock New is
// wired with outside tests.
type SystemClock struct{}

// NewClock returns a clock backed by the real wall clock.
func NewClock() *SystemClock {
	return &SystemClock{}
}

// Now returns the current time.
func (SystemClock) Now() time.Time {
	return time.Now()
}
