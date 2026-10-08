package recorder

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/ntpestimator"
	"github.com/bluenviron/mediamtx/internal/recordstore"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestFormatFMP4TrackNTPReanchor(t *testing.T) {
	for _, ca := range []string{
		"h265", "h264", "audio", "audio_video", "reordered_video", "reordered_audio_video",
		"backward_ntp", "audio_corrected_first", "video_corrected_first", "regular_rollover",
		"estimator", "estimator_audio_video", "keyframe", "no_drift", "source_ntp", "close_error",
	} {
		t.Run(ca, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var files []string
				ri := &recorderInstance{
					stream:            &stream.Stream{ReplaceNTP: ca != "source_ntp"},
					streamID:          uuid.New(),
					partDuration:      250 * time.Millisecond,
					segmentDuration:   time.Minute,
					maxPartSize:       1 << 20,
					pathFormat2:       filepath.Join(t.TempDir(), "%Y-%m-%d_%H-%M-%S-%f.mp4"),
					onSegmentCreate:   func(string) {},
					onSegmentComplete: func(path string, _ time.Duration) { files = append(files, path) },
					parent:            test.NilLogger,
				}
				if ca == "regular_rollover" {
					ri.segmentDuration = time.Second
				}
				if ca == "close_error" {
					blocker := filepath.Join(t.TempDir(), "file")
					require.NoError(t, os.WriteFile(blocker, []byte{1}, 0o600))
					ri.pathFormat2 = filepath.Join(blocker, "%f.mp4")
					ri.partDuration = time.Hour
				}
				initialStreamID := ri.streamID
				f := &formatFMP4{ri: ri}
				t.Cleanup(f.close)
				addTrack := func(codec mcodecs.Codec, clockRate uint32) {
					track := &formatFMP4Track{f: f, id: len(f.tracks) + 1, clockRate: clockRate, codec: codec}
					track.initialize()
					f.tracks = append(f.tracks, track)
				}
				switch ca {
				case "audio":
					addTrack(&mcodecs.Opus{ChannelCount: 2}, 48000)
				case "h264", "reordered_video", "reordered_audio_video":
					addTrack(&mcodecs.H264{SPS: test.FormatH264.SPS, PPS: test.FormatH264.PPS}, 90000)
				default:
					addTrack(&mcodecs.H265{
						VPS: test.FormatH265.VPS, SPS: test.FormatH265.SPS, PPS: test.FormatH265.PPS,
					}, 90000)
				}
				switch ca {
				case "audio_video", "reordered_audio_video", "audio_corrected_first", "video_corrected_first",
					"estimator_audio_video":
					addTrack(&mcodecs.Opus{ChannelCount: 2}, 48000)
				}

				frames := 65
				if ca == "estimator" || ca == "estimator_audio_video" {
					frames = 750
				}
				start := time.Now()
				estimators := make([]*ntpestimator.Estimator, len(f.tracks))
				timestamps := make([][]time.Time, len(f.tracks))
				offsets := make([][]int32, len(f.tracks))
				for j, track := range f.tracks {
					estimators[j] = &ntpestimator.Estimator{ClockRate: int(track.clockRate)}
				}
				for i := range frames {
					if ca == "estimator" || ca == "estimator_audio_video" {
						// Deliver at 10 fps while media timestamps advance at 12 fps.
						// The real estimator corrects itself twice, without clock jumps.
						time.Sleep(time.Until(start.Add(time.Duration(i) * time.Second / 10)))
					}
					for j, track := range f.tracks {
						step := int64(track.clockRate) / 12
						dts := int64(i) * step
						var offset int32
						if (ca == "reordered_video" || ca == "reordered_audio_video") && j == 0 && i%3 == 1 {
							offset = int32(2 * step)
						}
						ntp := start.Add(timestampToDuration(dts+int64(offset), int(track.clockRate)))
						correctionAt := 7
						if ca == "keyframe" {
							correctionAt = 12
						}
						if (ca == "audio_corrected_first" && j == 0) || (ca == "video_corrected_first" && j == 1) {
							correctionAt = 10
						}
						if ca != "no_drift" {
							correction := time.Duration(0)
							if i >= correctionAt {
								correction = 6 * time.Second
							}
							if i >= 40 {
								correction += 6 * time.Second
							}
							if ca == "backward_ntp" {
								correction = -correction
							}
							ntp = ntp.Add(correction)
						}
						if ca == "estimator" || ca == "estimator_audio_video" {
							ntp = estimators[j].Estimate(dts)
						}
						timestamps[j] = append(timestamps[j], ntp)
						offsets[j] = append(offsets[j], offset)
						payload := make([]byte, 4)
						binary.BigEndian.PutUint32(payload, uint32(i))
						err := track.write(&formatFMP4Sample{
							Sample: &fmp4.Sample{
								Payload:         payload,
								IsNonSyncSample: track.codec.IsVideo() && i%12 != 0,
								PTSOffset:       offset,
							},
							dts: dts,
							ntp: ntp,
						})
						if ca == "source_ntp" && i == correctionAt+1 {
							require.EqualError(t, err, "detected drift between recording duration and absolute time, resetting")
							return
						}
						if ca == "close_error" && i == 12 {
							require.Error(t, err)
							require.NotContains(t, err.Error(), "detected drift")
							require.Equal(t, initialStreamID, ri.streamID)
							return
						}
						require.NoError(t, err)
					}
				}
				require.NoError(t, f.currentSegment.close())
				f.currentSegment = nil
				if ca == "no_drift" {
					require.Len(t, files, 1)
				} else {
					require.GreaterOrEqual(t, len(files), 3)
				}

				counts := make([]int, len(f.tracks))
				var previousMeta *recordstore.Mtxi
				for number, path := range files {
					b, err := os.ReadFile(path)
					require.NoError(t, err)
					var init fmp4.Init
					require.NoError(t, init.Unmarshal(bytes.NewReader(b)))
					meta := init.UserData[0].(*recordstore.Mtxi)
					if previousMeta == nil {
						require.Equal(t, [16]byte(initialStreamID), meta.StreamID)
					} else {
						drift := time.Duration(meta.NTP-previousMeta.NTP) - time.Duration(meta.DTS-previousMeta.DTS)
						if drift < -ntpDriftTolerance || drift > ntpDriftTolerance {
							require.NotEqual(t, previousMeta.StreamID, meta.StreamID)
						} else {
							require.Equal(t, previousMeta.StreamID, meta.StreamID)
						}
					}
					previousMeta = meta
					require.Equal(t, uint64(number), meta.SegmentNumber)
					// The segment uses the timestamp of the oldest queued sample.
					first := int((meta.DTS*12 + int64(time.Second)/2) / int64(time.Second))
					matchesTimestamp := false
					for j := range f.tracks {
						if timestamps[j][first].UnixNano() == meta.NTP {
							matchesTimestamp = true
						}
					}
					require.True(t, matchesTimestamp, "segment timestamp must match a queued sample")
					var parts fmp4.Parts
					require.NoError(t, parts.Unmarshal(b))
					seen := make([]bool, len(f.tracks))
					for _, part := range parts {
						for _, partTrack := range part.Tracks {
							j := partTrack.ID - 1
							track := f.tracks[j]
							step := int64(track.clockRate) / 12
							dts := int64(partTrack.BaseTime) + multiplyAndDivide(meta.DTS, int64(track.clockRate), int64(time.Second))
							if !seen[j] && track.codec.IsVideo() {
								require.False(t, partTrack.Samples[0].IsNonSyncSample)
							}
							seen[j] = true
							for _, sample := range partTrack.Samples {
								require.Equal(t, uint32(counts[j]), binary.BigEndian.Uint32(sample.Payload))
								require.Equal(t, uint32(step), sample.Duration)
								require.Equal(t, offsets[j][counts[j]], sample.PTSOffset)
								require.InDelta(t, int64(counts[j])*step, dts, 2)
								dts += int64(sample.Duration)
								counts[j]++
							}
						}
					}
				}
				for _, count := range counts {
					require.Equal(t, frames-1, count) // one normal lookahead sample per track
				}
			})
		})
	}
}
