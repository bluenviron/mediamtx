package recorder

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/recordstore"
)

type clockRecordingAnalysis struct {
	audioIDs  map[uint32]bool
	videoIDs  map[uint32]bool
	streamIDs map[[16]byte]bool
	metas     []recordstore.Mtxi
}

// analyzeClockRecording validates tagged synthetic PCM/video without parsing a
// real codec payload. Audio has a fixed sample count; video advances by 40ms.
func analyzeClockRecording(
	t *testing.T,
	paths []string,
	audioStart time.Duration,
	audioSamples int,
) clockRecordingAnalysis {
	t.Helper()
	result := clockRecordingAnalysis{
		audioIDs: make(map[uint32]bool), videoIDs: make(map[uint32]bool),
		streamIDs: make(map[[16]byte]bool), metas: make([]recordstore.Mtxi, 0, len(paths)),
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var init fmp4.Init
		require.NoError(t, init.Unmarshal(bytes.NewReader(data)))
		meta := *init.UserData[0].(*recordstore.Mtxi)
		result.metas = append(result.metas, meta)
		result.streamIDs[meta.StreamID] = true
		var parts fmp4.Parts
		require.NoError(t, parts.Unmarshal(data))
		firstVideo := true
		for _, part := range parts {
			for _, track := range part.Tracks {
				rate := int64(8000)
				if track.ID == 1 {
					rate = 90000
				}
				// Invert the container base on the original integer sample grid.
				ticks := int64(track.BaseTime) + (meta.DTS*rate+int64(time.Second)-1)/int64(time.Second)
				for _, sample := range track.Samples {
					id := binary.BigEndian.Uint32(sample.Payload)
					if track.ID == 2 {
						require.False(t, result.audioIDs[id], "duplicate audio %d", id)
						result.audioIDs[id] = true
						require.True(t, bytes.Equal(clockPayload(id, audioSamples*2), sample.Payload))
						require.Equal(t, uint32(audioSamples), sample.Duration)
						expected := int64(audioStart)*rate/int64(time.Second) + int64(id)*int64(audioSamples)
						require.Equal(t, expected, ticks, "audio DTS %d", id)
					} else {
						require.False(t, result.videoIDs[id], "duplicate video %d", id)
						result.videoIDs[id] = true
						require.True(t, bytes.Equal(clockPayload(id, 4), sample.Payload))
						require.Equal(t, uint32(3600), sample.Duration)
						require.Equal(t, int64(50*90000)+int64(id)*3600, ticks, "video DTS %d", id)
						if firstVideo {
							require.False(t, sample.IsNonSyncSample, "segment must start video on a keyframe")
							firstVideo = false
						}
					}
					ticks += int64(sample.Duration)
				}
			}
		}
	}
	return result
}
