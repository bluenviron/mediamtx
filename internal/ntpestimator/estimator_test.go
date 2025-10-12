package ntpestimator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEstimator(t *testing.T) {
	e := &Estimator{ClockRate: 90000}

	timeNow = func() time.Time { return time.Date(2003, 11, 4, 23, 15, 7, 0, time.UTC) }
	ntp := e.Estimate(90000)
	require.Equal(t, time.Date(2003, 11, 4, 23, 15, 7, 0, time.UTC), ntp)

	timeNow = func() time.Time { return time.Date(2003, 11, 4, 23, 15, 8, 0, time.UTC) }
	ntp = e.Estimate(2 * 90000)
	require.Equal(t, time.Date(2003, 11, 4, 23, 15, 8, 0, time.UTC), ntp)

	timeNow = func() time.Time { return time.Date(2003, 11, 4, 23, 15, 10, 0, time.UTC) }
	ntp = e.Estimate(3 * 90000)
	require.Equal(t, time.Date(2003, 11, 4, 23, 15, 9, 0, time.UTC), ntp)

	timeNow = func() time.Time { return time.Date(2003, 11, 4, 23, 15, 9, 0, time.UTC) }
	ntp = e.Estimate(4 * 90000)
	require.Equal(t, time.Date(2003, 11, 4, 23, 15, 9, 0, time.UTC), ntp)

	timeNow = func() time.Time { return time.Date(2003, 11, 4, 23, 15, 15, 0, time.UTC) }
	ntp = e.Estimate(5 * 90000)
	require.Equal(t, time.Date(2003, 11, 4, 23, 15, 10, 0, time.UTC), ntp)

	timeNow = func() time.Time { return time.Date(2003, 11, 4, 23, 15, 20, 0, time.UTC) }
	ntp = e.Estimate(6 * 90000)
	require.Equal(t, time.Date(2003, 11, 4, 23, 15, 20, 0, time.UTC), ntp)
}
