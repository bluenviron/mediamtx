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

type pauseRecordingAnalysis struct {
	audioIDs    map[uint32]bool
	videoIDs    map[uint32]bool
	badDuration int
	badDTS      int
}

// analyzePauseRecording checks payload identities, original DTS, and keyframe boundaries.
// Only the pre-pause packet in an unclosed segment may represent the exact source gap.
func analyzePauseRecording(
	t *testing.T,
	paths []string,
	lead time.Duration,
	pauseStart, pauseEnd int,
	crossedPause bool,
) pauseRecordingAnalysis {
	t.Helper()
	result := pauseRecordingAnalysis{
		audioIDs: make(map[uint32]bool),
		videoIDs: make(map[uint32]bool),
	}
	for _, path := range paths {
		data, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		var init fmp4.Init
		require.NoError(t, init.Unmarshal(bytes.NewReader(data)))
		meta := init.UserData[0].(*recordstore.Mtxi)
		var parts fmp4.Parts
		require.NoError(t, parts.Unmarshal(data))
		firstVideo := true
		for _, part := range parts {
			for _, tr := range part.Tracks {
				dts := int64(tr.BaseTime)
				for _, s := range tr.Samples {
					id := binary.BigEndian.Uint32(s.Payload)
					expected := 50*time.Second + time.Duration(id)*40*time.Millisecond
					rate := 90000
					if tr.ID == 2 {
						require.False(t, result.audioIDs[id], "duplicate audio %d", id)
						result.audioIDs[id] = true
						expected += lead
						rate = 8000
						sourceGapDuration := uint32(pauseEnd-pauseStart+1) * 320
						isUnclosedSourceGap := id == uint32(pauseStart-1) && !crossedPause && s.Duration == sourceGapDuration
						if s.Duration != 320 && !isUnclosedSourceGap {
							result.badDuration++
							t.Logf("bad duration audio id=%d duration=%v", id, timestampToDuration(int64(s.Duration), 8000))
						}
					} else {
						require.False(t, result.videoIDs[id], "duplicate video %d", id)
						result.videoIDs[id] = true
						if firstVideo {
							require.False(t, s.IsNonSyncSample, "segment video must start with IDR")
							firstVideo = false
						}
					}
					actual := time.Duration(meta.DTS) + timestampToDuration(dts, rate)
					if (actual - expected).Abs() > 125*time.Microsecond {
						result.badDTS++
						t.Logf("track=%d id=%d actualDTS=%v want=%v", tr.ID, id, actual, expected)
					}
					dts += int64(s.Duration)
				}
			}
		}
	}

	return result
}
