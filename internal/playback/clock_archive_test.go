package playback

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/recordstore"
)

func TestClockCorrectionArchivePlaybackMetadata(t *testing.T) {
	dir := os.Getenv("MTX_CLOCK_REPLAY_DIR")
	if dir == "" {
		t.Skip("set MTX_CLOCK_REPLAY_DIR to the retained 100-minute archive")
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.mp4"))
	require.NoError(t, err)
	type segment struct {
		init     *fmp4.Init
		meta     *recordstore.Mtxi
		duration time.Duration
	}
	segments := make([]segment, 0, len(paths))
	for _, path := range paths {
		f, openErr := os.Open(path)
		require.NoError(t, openErr)
		init, duration, readErr := recordstore.SegmentFMP4ReadHeader(f)
		require.NoError(t, readErr)
		require.NoError(t, f.Close())
		meta := findMtxi(init.UserData)
		require.NotNil(t, meta)
		segments = append(segments, segment{init, meta, duration})
	}
	sort.Slice(segments, func(i, j int) bool {
		return segments[i].meta.SegmentNumber < segments[j].meta.SegmentNumber
	})
	require.Greater(t, len(segments), 90)
	joins, breaks := 0, 0
	for i := 1; i < len(segments); i++ {
		prev, current := segments[i-1], segments[i]
		require.Equal(t, prev.meta.SegmentNumber+1, current.meta.SegmentNumber)
		start := time.Unix(0, current.meta.NTP)
		prevEnd := time.Unix(0, prev.meta.NTP).Add(prev.duration)
		canJoin := segmentFMP4CanBeConcatenated(prev.init, prevEnd, current.init, start)
		require.Equal(t, prev.meta.StreamID == current.meta.StreamID, canJoin)
		if canJoin {
			joins++
		} else {
			breaks++
		}
	}
	require.GreaterOrEqual(t, breaks, 3)
	require.Equal(t, len(segments)-1, joins+breaks)
	t.Logf("actual playback concatenation policy: files=%d same-UUID joins=%d correction-range breaks=%d",
		len(segments), joins, breaks)
}
