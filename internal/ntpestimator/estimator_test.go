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

func TestEstimatorShared(t *testing.T) {
	at := func(sec, nsec int) func() time.Time {
		return func() time.Time { return time.Date(2003, 11, 4, 23, 15, sec, nsec, time.UTC) }
	}

	var s Shared

	// video ahead of the wall clock re-anchors the estimate
	timeNow = at(0, 0)
	s.Estimate(0, 90000)
	timeNow = at(1, 0)
	s.Estimate(90000, 90000)
	timeNow = at(1, 500000000)
	video := s.Estimate(180000, 90000)

	// other formats with the same timestamp get the same NTP
	timeNow = at(1, 510000000)
	klv := s.Estimate(180000, 90000)
	audio := s.Estimate(96000, 48000)

	require.Equal(t, video, klv)
	require.Equal(t, video, audio)
}
