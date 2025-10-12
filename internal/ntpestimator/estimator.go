// Package ntpestimator contains a NTP estimator.
package ntpestimator

import (
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

// Estimate returns estimated NTP.
func (e *Estimator) Estimate(pts int64) time.Time {
	now := timeNow()

	if e.refNTP.Equal(zero) {
		e.refNTP = now
		e.refPTS = pts
		return now
	}

	computed := e.refNTP.Add((multiplyAndDivide(time.Duration(pts-e.refPTS), time.Second, time.Duration(e.ClockRate))))

	if computed.After(now) || computed.Before(now.Add(-maxTimeDifference)) {
		e.refNTP = now
		e.refPTS = pts
		return now
	}

	return computed
}
