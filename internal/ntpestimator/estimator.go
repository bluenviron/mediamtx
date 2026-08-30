// Package ntpestimator contains a NTP estimator.
package ntpestimator

import (
	"time"
)

// berry's, reduced from upstream's 5 seconds for tighter drift detection
var ( // var so we can restore it to 5 seconds in test
	maxTimeDifference = 1 * time.Second
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
