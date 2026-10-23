package recordstore_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/recordstore"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestSegmentFMP4ReadHeaderAndDuration(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(map[bool]string{false: "parts", true: "header"}[cached], func(t *testing.T) {
			fpath := filepath.Join(t.TempDir(), "segment.mp4")
			test.WriteRecordingSegment(t, fpath, 2250*time.Millisecond, cached)

			init, duration, err := recordstore.SegmentFMP4ReadHeaderAndDuration(fpath)
			require.NoError(t, err)
			require.Len(t, init.Tracks, 1)
			require.Equal(t, 2250*time.Millisecond, duration)
		})
	}
}

func TestSegmentFMP4ReadHeaderAndDurationInvalid(t *testing.T) {
	fpath := filepath.Join(t.TempDir(), "segment.mp4")

	_, _, err := recordstore.SegmentFMP4ReadHeaderAndDuration(fpath)
	require.Error(t, err)

	err = os.WriteFile(fpath, []byte{}, 0o644)
	require.NoError(t, err)

	_, _, err = recordstore.SegmentFMP4ReadHeaderAndDuration(fpath)
	require.Error(t, err)
}

func TestSegmentFMP4ReadHeader(t *testing.T) {
	fpath := filepath.Join(t.TempDir(), "segment.mp4")
	test.WriteRecordingSegment(t, fpath, 1500*time.Millisecond, true)

	buf, err := os.ReadFile(fpath)
	require.NoError(t, err)

	init, duration, err := recordstore.SegmentFMP4ReadHeader(bytes.NewReader(buf))
	require.NoError(t, err)
	require.Len(t, init.Tracks, 1)
	require.Equal(t, 1500*time.Millisecond, duration)
}
