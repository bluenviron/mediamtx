package h264

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDTSEstimator(t *testing.T) {
	est := NewDTSEstimator()

	dts := est.Feed(false, 2*time.Second)
	require.Equal(t, time.Millisecond, dts)

	dts = est.Feed(false, 2*time.Second-200*time.Millisecond)
	require.Equal(t, 2*time.Millisecond, dts)

	dts = est.Feed(false, 2*time.Second-400*time.Millisecond)
	require.Equal(t, 3*time.Millisecond, dts)

	dts = est.Feed(false, 2*time.Second+200*time.Millisecond)
	require.Equal(t, 2*time.Second-400*time.Millisecond, dts)

	dts = est.Feed(true, 2*time.Second+300*time.Millisecond)
	require.Equal(t, 2*time.Second+300*time.Millisecond, dts)
}
