package playback

import (
	"bytes"
	"testing"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

// Test that tracks with no samples are not included in the output
func TestMuxerMP4EmptyTracks(t *testing.T) {
	var buf bytes.Buffer

	mux := &muxerMP4{
		w: &buf,
	}

	init := &fmp4.Init{
		Tracks: []*fmp4.InitTrack{
			{
				ID:        1,
				TimeScale: 90000,
				Codec: &mcodecs.H264{
					SPS: test.FormatH264.SPS,
					PPS: test.FormatH264.PPS,
				},
			},
			{
				ID:        2,
				TimeScale: 48000,
				Codec:     &mcodecs.Opus{},
			},
		},
	}

	mux.writeInit(init)

	// Only write samples to the first track
	mux.setTrack(1)
	err := mux.writeSample(0, 0, false, 5, func() ([]byte, error) {
		return []byte{0x01, 0x02, 0x03, 0x04, 0x05}, nil
	})
	require.NoError(t, err)

	err = mux.writeSample(90000, 0, true, 5, func() ([]byte, error) {
		return []byte{0x06, 0x07, 0x08, 0x09, 0x0a}, nil
	})
	require.NoError(t, err)

	mux.writeFinalDTS(180000)

	err = mux.flush()
	require.NoError(t, err)

	require.Greater(t, buf.Len(), 0)
}
