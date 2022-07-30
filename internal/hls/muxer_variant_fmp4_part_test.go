package hls

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDurationGoToMp4(t *testing.T) {
	require.Equal(t, int64(9000000000), durationGoToMp4(10000000000000, 900000))
	require.Equal(t, int64(90000000000), durationGoToMp4(100000000000000, 900000))
}
