package recorder

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestRecorderLPCMResumeBoundary(t *testing.T) {
	for _, stall := range []time.Duration{50 * time.Second} {
		for _, lead := range []time.Duration{-1500 * time.Millisecond} {
			for _, audioFirst := range []bool{true, false} {
				name := fmt.Sprintf("stall=%v/lead=%v/audioFirst=%v", stall, lead, audioFirst)
				t.Run(name, func(t *testing.T) {
					dir := t.TempDir()
					late := 0
					created := map[string]bool{}
					closedCount, closedAtPause := 0, 0
					crossedPause := false
					f := &formatFMP4{
						ri: &recorderInstance{
							pathFormat2:     filepath.Join(dir, "%Y-%m-%d_%H-%M-%S-%f.mp4"),
							location:        time.UTC,
							partDuration:    100 * time.Millisecond,
							maxPartSize:     50 * 1024 * 1024,
							segmentDuration: 60 * time.Second,

							parent: test.Logger(func(_ logger.Level, format string, _ ...any) {
								if format == "sample of track %d received too late, discarding" {
									late++
								}
							}),
							onSegmentCreate: func(path string) {
								require.False(t, created[path], "filename reused")
								created[path] = true
							},
							onSegmentComplete: func(string, time.Duration) {
								closedCount++
							},
						},
					}
					v := &formatFMP4Track{
						f:         f,
						id:        1,
						clockRate: 90000,
						codec: &mcodecs.H264{
							SPS: test.FormatH264.SPS,
							PPS: test.FormatH264.PPS,
						},
					}
					a := &formatFMP4Track{
						f:         f,
						id:        2,
						clockRate: 8000,
						codec: &mcodecs.LPCM{
							SampleRate:   8000,
							BitDepth:     16,
							ChannelCount: 1,
						},
					}
					v.initialize()
					a.initialize()
					f.tracks = []*formatFMP4Track{v, a}
					ntp := time.Unix(1700000000, 0)
					count := 3500
					pauseEnd := 1480
					pauseStart := pauseEnd - int(stall/(40*time.Millisecond))
					expectedAudio := map[uint32]bool{}
					for i := range count {
						at := time.Duration(i) * 40 * time.Millisecond
						wa := func() {
							if i == pauseStart-1 {
								closedAtPause = closedCount
							}
							if i >= pauseStart && i < pauseEnd {
								return
							}
							if i == pauseEnd {
								crossedPause = closedCount > closedAtPause
							}
							payload := make([]byte, 640)
							binary.BigEndian.PutUint32(payload, uint32(i))
							require.NoError(t,
								a.write(&formatFMP4Sample{
									Sample: &fmp4.Sample{
										Payload: payload,
									},
									dts: int64((50*time.Second+at+lead)/time.Millisecond) * 8,
									ntp: ntp.Add(at),
								}))
							if i < count-1 {
								expectedAudio[uint32(i)] = true
							}
						}
						wv := func() {
							payload := make([]byte, 4)
							binary.BigEndian.PutUint32(payload, uint32(i))
							require.NoError(t,
								v.write(&formatFMP4Sample{
									Sample: &fmp4.Sample{
										Payload:         payload,
										IsNonSyncSample: i%25 != 0,
									},
									dts: int64((50*time.Second+at)/time.Millisecond) * 90,
									ntp: ntp.Add(at),
								}))
						}
						if audioFirst {
							wa()
							wv()
						} else {
							wv()
							wa()
						}
					}
					f.close()
					paths, err := filepath.Glob(filepath.Join(dir, "*.mp4"))
					require.NoError(t, err)
					require.GreaterOrEqual(t, len(paths), 3)
					analysis := analyzePauseRecording(t, paths, lead, pauseStart, pauseEnd, crossedPause)
					missing := []uint32{}
					for id := range expectedAudio {
						if !analysis.audioIDs[id] {
							missing = append(missing, id)
						}
					}
					t.Logf("crossedPause=%v received audio=%d retained=%d missing=%v video=%d/%d "+
						"badDTS=%d badDuration=%d late=%d",
						crossedPause,
						len(expectedAudio),
						len(analysis.audioIDs),
						missing,
						len(analysis.videoIDs),
						count-1,
						analysis.badDTS,
						analysis.badDuration,
						late)
					if late != 0 {
						t.Errorf("received-too-late warnings=%d", late)
					}
					require.Empty(t, missing)
					require.Len(t, analysis.audioIDs, len(expectedAudio))
					require.Len(t, analysis.videoIDs, count-1)
					require.Zero(t, analysis.badDTS)
					require.Zero(t, analysis.badDuration)
					require.True(t, analysis.audioIDs[uint32(pauseStart-1)], "held pre-pause audio lost")
					require.True(t, analysis.audioIDs[uint32(pauseEnd)], "first resumed audio lost")
				})
			}
		}
	}
}
