package recorder

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/recordstore"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestRecorderLocalNTPFreshnessWithClockDrift(t *testing.T) {
	for _, localMode := range []bool{true, false} {
		for _, model := range []struct {
			name    string
			ppm     int64
			lag     time.Duration
			slowDTS bool
		}{
			{"control_fixed_offset", 0, 1500 * time.Millisecond, false},
			{"NTP_only_550ppm", 550, 1500 * time.Millisecond, false},
			{"coupled_DTS_NTP_550ppm", 550, 0, true},
		} {
			t.Run(model.name+map[bool]string{true: "/local", false: "/absolute"}[localMode], func(t *testing.T) {
				dir := clockEvidenceDir(t)
				ntp := time.Unix(1700000000, 0)
				wall := time.Duration(0)
				late := 0
				firstLate := time.Duration(-1)
				falseStale := 0
				maxLookback := time.Duration(0)
				created := map[string]bool{}
				f := &formatFMP4{ri: &recorderInstance{
					pathFormat2: filepath.Join(dir,
						"%Y-%m-%d_%H-%M-%S-%f.mp4"),
					location:        time.UTC,
					partDuration:    time.Second,
					maxPartSize:     50 * 1024 * 1024,
					segmentDuration: time.Minute,

					parent: test.Logger(func(_ logger.Level, format string, args ...any) {
						if format == "sample of track %d received too late, discarding" {
							require.Equal(t, 2, args[0].(int))
							late++
							if firstLate < 0 {
								firstLate = wall
								t.Logf("FIRST LATE receipt=%v", wall)
							}
						}
					}),
					onSegmentCreate: func(path string) {
						require.False(t,
							created[path],
							"filename reused")
						created[path] = true
					},
					onSegmentComplete: func(string,
						time.Duration) {
					},
				}}
				f.ri.stream = &stream.Stream{ReplaceNTP: localMode}
				f.now = func() time.Time { return ntp.Add(wall) }
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
				const count = 108050 // 36m1s, every received audio packet remains present in the input.
				audioDTS := func(id int) int64 {
					ticks := int64(id) * 160
					if model.slowDTS {
						ticks = ticks * (1000000 - model.ppm) / 1000000
					}
					return 50*8000 - int64(model.lag/time.Millisecond)*8 + ticks
				}
				for i := range count {
					wall = time.Duration(i) * 20 * time.Millisecond
					if i%2 == 0 {
						before := f.currentSegment
						written := a.nextSampleWritten
						payload := make([]byte, 4)
						binary.BigEndian.PutUint32(payload, uint32(i/2))
						require.NoError(t,
							v.write(&formatFMP4Sample{
								Sample: &fmp4.Sample{
									Payload:         payload,
									IsNonSyncSample: i%50 != 0,
								},
								dts: 50*90000 + int64(i)*1800,
								ntp: ntp.Add(wall),
							}))
						if f.currentSegment != before {
							lookback := timestampToDuration(v.nextSample.dts, 90000) - f.currentSegment.startDTS
							maxLookback = max(maxLookback, lookback)
							if a.nextSampleWritten && !written {
								falseStale++
							}
							if wall >= 29*time.Minute && wall <= 33*time.Minute && a.nextSample != nil {
								t.Logf("BOUNDARY receipt=%v receiptAge=20ms audioNTPage=%v audioDTSLag=%v "+
									"baseLookback=%v pendingCommitted=%v",
									wall,
									ntp.Add(wall).Sub(a.nextSample.ntp),
									timestampToDuration(v.nextSample.dts,
										90000)-timestampToDuration(a.nextSample.dts,
										8000),
									lookback,
									a.nextSampleWritten)
							}
						}
					}
					payload := make([]byte, 320)
					binary.BigEndian.PutUint32(payload, uint32(i))
					slowTicks := int64(i) * 160 * (1000000 - model.ppm) / 1000000
					require.NoError(t,
						a.write(&formatFMP4Sample{
							Sample: &fmp4.Sample{
								Payload: payload,
							},
							dts: audioDTS(i),
							ntp: ntp.Add(timestampToDuration(slowTicks,
								8000)),
						}))
				}
				f.close()
				paths, err := filepath.Glob(filepath.Join(dir, "*.mp4"))
				require.NoError(t, err)
				seenAudio := map[uint32]bool{}
				seenVideo := map[uint32]bool{}
				badDTS := 0
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
							ticks := int64(tr.BaseTime)
							for _, sample := range tr.Samples {
								id := binary.BigEndian.Uint32(sample.Payload)
								rate := 8000
								expected := timestampToDuration(audioDTS(int(id)), 8000)
								if tr.ID == 2 {
									require.False(t, seenAudio[id], "duplicate audio %d", id)
									seenAudio[id] = true
								} else {
									rate = 90000
									expected = 50*time.Second + time.Duration(id)*40*time.Millisecond
									require.False(t, seenVideo[id])
									seenVideo[id] = true
									if firstVideo {
										require.False(t, sample.IsNonSyncSample, "first video must be keyframe")
										firstVideo = false
									}
								}
								actual := time.Duration(meta.DTS) + timestampToDuration(ticks, rate)
								if (actual - expected).Abs() > time.Second/time.Duration(rate)+1 {
									badDTS++
								}
								ticks += int64(sample.Duration)
							}
						}
					}
				}
				t.Logf("SUMMARY model=%s elapsed=%v firstLate=%v late=%d receivedAudio=%d "+
					"retainedAudio=%d receivedVideo=%d retainedVideo=%d falseStaleCommits=%d "+
					"maxLookback=%v badDTS=%d",
					model.name,
					wall,
					firstLate,
					late,
					count-1,
					len(seenAudio),
					count/2-1,
					len(seenVideo),
					falseStale,
					maxLookback,
					badDTS)
				require.Zero(t, badDTS)
				require.Len(t, seenVideo, count/2-1)
				require.LessOrEqual(t, maxLookback, 5*time.Second)
				require.Equal(t, count-1-late, len(seenAudio))
				if localMode || model.ppm == 0 {
					require.Len(t, seenAudio, count-1, "all received audio must survive")
					require.Zero(t, late)
					require.Zero(t, falseStale)
				} else {
					require.Equal(t, 35, late, "external-clock policy must remain unchanged")
				}
			})
		}
	}
}
