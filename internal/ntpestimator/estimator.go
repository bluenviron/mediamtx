// Package ntpestimator contains a NTP estimator.
package ntpestimator

import (
	"sync"
	"time"
)

const (
	maxTimeDifference = 5 * time.Second
)

var timeNow = time.Now

func multiplyAndDivide(v, m, d time.Duration) time.Duration {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

// Estimator is a NTP estimator.
type Estimator struct {
	ClockRate int

	shared Shared
}

// Estimate returns estimated NTP.
func (e *Estimator) Estimate(pts int64) time.Time {
	return e.shared.Estimate(pts, e.ClockRate)
}

// Shared is a NTP estimator shared by formats with a common timeline.
type Shared struct {
	mutex  sync.Mutex
	refNTP time.Time
	refPTS time.Duration
}

// Estimate returns estimated NTP.
func (s *Shared) Estimate(pts int64, clockRate int) time.Time {
	d := multiplyAndDivide(time.Duration(pts), time.Second, time.Duration(clockRate))

	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := timeNow()

	// do not store monotonic clock, in order to include
	// system clock changes into time differences
	now = now.Round(0)

	if s.refNTP.IsZero() {
		s.refNTP = now
		s.refPTS = d
		return now
	}

	computed := s.refNTP.Add(d - s.refPTS)

	if computed.After(now) || computed.Before(now.Add(-maxTimeDifference)) {
		s.refNTP = now
		s.refPTS = d
		return now
	}

	return computed
}
