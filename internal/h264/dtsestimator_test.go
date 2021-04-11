package h264

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDTSEstimator(t *testing.T) {
	est := NewDTSEstimator()
	est.Feed(2 * time.Second)
	est.Feed(2*time.Second - 200*time.Millisecond)
	est.Feed(2*time.Second - 400*time.Millisecond)
	dts := est.Feed(2*time.Second + 200*time.Millisecond)
	require.Equal(t, 2*time.Second-400*time.Millisecond, dts)
}
