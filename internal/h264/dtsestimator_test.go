package h264

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDTSEstimator(t *testing.T) {
	est := NewDTSEstimator()

	// initial state
	dts := est.Feed(0)
	require.Equal(t, time.Duration(0), dts)

	// b-frame
	dts = est.Feed(1*time.Second - 200*time.Millisecond)
	require.Equal(t, time.Millisecond, dts)

	// b-frame
	dts = est.Feed(1*time.Second - 400*time.Millisecond)
	require.Equal(t, 2*time.Millisecond, dts)

	// p-frame
	dts = est.Feed(1 * time.Second)
	require.Equal(t, 1*time.Second-400*time.Millisecond, dts)

	// p-frame
	dts = est.Feed(1*time.Second + 200*time.Millisecond)
	require.Equal(t, 1*time.Second-399*time.Millisecond, dts)
}
