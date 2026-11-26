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

// Audio lookback determines the container base, while video sync samples determine
// rotation cadence. A held audio packet must not select the same filename repeatedly.
func TestRecorderShortSegmentsPreservePayloadAndDTS(t *testing.T) {
	for _, lead := range []time.Duration{-1500 * time.Millisecond, 0, 1500 * time.Millisecond} {
		for _, delayed := range []bool{false, true} {
			t.Run(fmt.Sprintf("lead=%s/delay=%t", lead, delayed), func(t *testing.T) {
				dir := t.TempDir()
				created := make(map[string]bool)
				late := 0
				f := &formatFMP4{ri: &recorderInstance{
					pathFormat2:     filepath.Join(dir, "%Y-%m-%d_%H-%M-%S-%f.mp4"),
					location:        time.UTC,
					partDuration:    100 * time.Millisecond,
					maxPartSize:     50 * 1024 * 1024,
					segmentDuration: time.Second,
					parent: test.Logger(func(_ logger.Level, format string, _ ...any) {
						if format == "sample of track %d received too late, discarding" {
							late++
						}
					}),
					onSegmentCreate: func(path string) {
						require.False(t, created[path], "segment filename reused: %s", path)
						created[path] = true
					},
					onSegmentComplete: func(string, time.Duration) {},
				}}
				video := &formatFMP4Track{
					f: f, id: 1, clockRate: 90000,
					codec: &mcodecs.H264{SPS: test.FormatH264.SPS, PPS: test.FormatH264.PPS},
				}
				audio := &formatFMP4Track{
					f: f, id: 2, clockRate: 8000,
					codec: &mcodecs.LPCM{SampleRate: 8000, BitDepth: 16, ChannelCount: 1},
				}
				video.initialize()
				audio.initialize()
				f.tracks = []*formatFMP4Track{video, audio}
				ntp := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
				writeAudioAt := func(id int) {
					at := time.Duration(id) * 40 * time.Millisecond
					payload := make([]byte, 640)
					binary.BigEndian.PutUint32(payload, uint32(id))
					require.NoError(t, audio.write(&formatFMP4Sample{
						Sample: &fmp4.Sample{Payload: payload},
						dts:    int64((50*time.Second+at+lead)/time.Millisecond) * 8,
						ntp:    ntp.Add(at),
					}))
				}
				const count = 250
				for i := range count {
					at := time.Duration(i) * 40 * time.Millisecond
					writeAudio := func() {
						if delayed && i >= 45 && i < 60 {
							return
						}
						if delayed && i == 60 {
							for j := 45; j < 60; j++ {
								writeAudioAt(j)
							}
						}
						writeAudioAt(i)
					}
					writeVideo := func() {
						payload := make([]byte, 4)
						binary.BigEndian.PutUint32(payload, uint32(i))
						require.NoError(t, video.write(&formatFMP4Sample{
							Sample: &fmp4.Sample{Payload: payload, IsNonSyncSample: i%5 != 0},
							dts:    int64((50*time.Second+at)/time.Millisecond) * 90,
							ntp:    ntp.Add(at),
						}))
					}
					// Prime the earlier track; separate tests cover both startup orders.
					if lead <= 0 {
						writeAudio()
						writeVideo()
					} else {
						writeVideo()
						writeAudio()
					}
				}
				f.close()
				paths, err := filepath.Glob(filepath.Join(dir, "*.mp4"))
				require.NoError(t, err)
				require.Len(t, paths, 10, "one-second rotations must follow video time")
				require.Len(t, created, len(paths))
				// All delayed packets are delivered: no source-gap duration exception applies.
				analysis := analyzePauseRecording(t, paths, lead, 45, 60, true)
				require.Len(t, analysis.audioIDs, count-1)
				require.Len(t, analysis.videoIDs, count-1)
				for id := range count - 1 {
					require.True(t, analysis.audioIDs[uint32(id)], "missing audio %d", id)
					require.True(t, analysis.videoIDs[uint32(id)], "missing video %d", id)
				}
				require.Zero(t, analysis.badDuration)
				require.Zero(t, analysis.badDTS)
				require.Zero(t, late)
			})
		}
	}
}
