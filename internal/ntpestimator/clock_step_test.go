package ntpestimator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEstimatorWallClockStep(t *testing.T) {
	for _, step := range []time.Duration{0, 5100 * time.Millisecond, 24 * time.Hour, -24 * time.Hour} {
		t.Run(step.String(), func(t *testing.T) {
			saved := timeNow
			t.Cleanup(func() { timeNow = saved })
			start := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
			now := start
			timeNow = func() time.Time { return now }
			estimator := &Estimator{ClockRate: 90000}
			for i := range 8 {
				// Only wall time changes; the media clock continues at its original rate.
				now = start.Add(time.Duration(i) * time.Second)
				if i >= 3 {
					now = now.Add(step)
				}
				require.Equal(t, now, estimator.Estimate(int64(i)*90000))
			}
		})
	}
}
