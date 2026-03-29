package utils

import "time"

type Clock interface {
	NowUtc() time.Time
}

type RealClock struct{}

func NewRealClock() *RealClock {
	return &RealClock{}
}

func (rc *RealClock) NowUtc() time.Time {
	return time.Now().UTC()
}
