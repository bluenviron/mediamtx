// Package ntpestimator contains a NTP estimator.
package ntpestimator

import (
	"fmt"
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

	refNTP time.Time
	refPTS int64
}

var zero = time.Time{}

// Initialize initializes the estimator.
func (e *Estimator) Initialize() error {
	if e.ClockRate <= 0 {
		return fmt.Errorf("invalid clock rate: %d", e.ClockRate)
	}
	return nil
}

// Estimate returns estimated NTP.
func (e *Estimator) Estimate(pts int64) time.Time {
	now := timeNow()

	// do not store monotonic clock, in order to include
	// system clock changes into time differences
	now = now.Round(0)

	if e.refNTP.Equal(zero) {
		e.refNTP = now
		e.refPTS = pts
		return now
	}

	computed := e.refNTP.Add(multiplyAndDivide(time.Duration(pts-e.refPTS), time.Second, time.Duration(e.ClockRate)))

	if computed.After(now) || computed.Before(now.Add(-maxTimeDifference)) {
		e.refNTP = now
		e.refPTS = pts
		return now
	}

	return computed
}
