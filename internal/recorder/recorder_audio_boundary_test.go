package recorder

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/recordstore"
	"github.com/bluenviron/mediamtx/internal/test"
)

// Exercise the actual recorder and parse its files. Payload identifiers prove
// retention; mtxi + tfdt prove original DTS and continuous sample timing.
func TestRecorderAudioBoundaryPreservesPayloadAndDTS(t *testing.T) {
	for _, ca := range []struct {
		lead  time.Duration
		delay bool
	}{
		{-1500 * time.Millisecond, false},
		{0, false},
		{1500 * time.Millisecond, false},
		{-1500 * time.Millisecond, true},
		{0, true},
		{1500 * time.Millisecond, true},
	} {
		for _, audioFirst := range []bool{true, false} {
			lead := ca.lead
			t.Run(fmt.Sprintf("%s/delay=%v/audioFirst=%v", lead, ca.delay, audioFirst), func(t *testing.T) {
				dir := t.TempDir()
				f := &formatFMP4{ri: &recorderInstance{
					pathFormat2: filepath.Join(dir, "%Y-%m-%d_%H-%M-%S-%f.mp4"), location: time.UTC,
					partDuration: 100 * time.Millisecond, maxPartSize: 50 * 1024 * 1024, segmentDuration: 2 * time.Second,
					parent: test.NilLogger, onSegmentCreate: func(string) {}, onSegmentComplete: func(string, time.Duration) {},
				}}
				video := &formatFMP4Track{
					f:         f,
					id:        1,
					clockRate: 90000,
					codec: &mcodecs.H264{
						SPS: test.FormatH264.SPS,
						PPS: test.FormatH264.PPS,
					},
				}
				audio := &formatFMP4Track{
					f:         f,
					id:        2,
					clockRate: 8000,
					codec: &mcodecs.LPCM{
						SampleRate:   8000,
						BitDepth:     16,
						ChannelCount: 1,
					},
				}
				video.initialize()
				audio.initialize()
				f.tracks = []*formatFMP4Track{video, audio}
				ntp := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
				const count = 250
				for i := range count {
					at := time.Duration(i) * 40 * time.Millisecond
					payload := make([]byte, 640)
					binary.BigEndian.PutUint32(payload, uint32(i))
					writeAudio := func() {
						if ca.delay && i >= 45 && i < 60 {
							return
						}
						if ca.delay && i == 60 {
							for j := 45; j < 60; j++ {
								delayed := time.Duration(j) * 40 * time.Millisecond
								p := make([]byte, 640)
								binary.BigEndian.PutUint32(p, uint32(j))
								require.NoError(t,
									audio.write(&formatFMP4Sample{
										Sample: &fmp4.Sample{
											Payload: p,
										},
										dts: int64((50*time.Second+delayed+lead)/time.Millisecond) * 8,
										ntp: ntp.Add(delayed),
									}))
							}
						}
						require.NoError(t,
							audio.write(&formatFMP4Sample{
								Sample: &fmp4.Sample{
									Payload: payload,
								},
								dts: int64((50*time.Second+at+lead)/time.Millisecond) * 8,
								ntp: ntp.Add(at),
							}))
					}
					writeVideo := func() {
						require.NoError(t,
							video.write(&formatFMP4Sample{
								Sample: &fmp4.Sample{
									Payload: []byte{
										byte(i),
									},
									IsNonSyncSample: i%25 != 0,
								},
								dts: int64((50*time.Second+at)/time.Millisecond) * 90,
								ntp: ntp.Add(at),
							}))
					}
					// Exercise both arrival orders, including the first segment.
					if audioFirst {
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
				require.GreaterOrEqual(t, len(paths), 3)
				seen := make(map[uint32]bool)
				videos := 0
				seenVideo := make(map[byte]bool)
				for _, path := range paths {
					data, readErr := os.ReadFile(path)
					require.NoError(t, readErr)
					var init fmp4.Init
					require.NoError(t, init.Unmarshal(bytes.NewReader(data)))
					meta := init.UserData[0].(*recordstore.Mtxi)
					var parts fmp4.Parts
					require.NoError(t, parts.Unmarshal(data))
					videoFirst := true
					for _, part := range parts {
						for _, track := range part.Tracks {
							dts := int64(track.BaseTime)
							for _, sample := range track.Samples {
								switch track.ID {
								case 2:
									id := binary.BigEndian.Uint32(sample.Payload)
									require.False(t, seen[id], "duplicate audio %d", id)
									seen[id] = true
									expected := 50*time.Second + time.Duration(id)*40*time.Millisecond + lead
									actual := time.Duration(meta.DTS) + timestampToDuration(dts, 8000)
									require.InDelta(t, int64(expected), int64(actual), 125000, "audio %d DTS shifted", id)
									require.Equal(t, uint32(320), sample.Duration)
								case 1:
									id := sample.Payload[0]
									require.False(t, seenVideo[id], "duplicate video %d", id)
									seenVideo[id] = true
									expected := 50*time.Second + time.Duration(id)*40*time.Millisecond
									actual := time.Duration(meta.DTS) + timestampToDuration(dts, 90000)
									require.InDelta(t, int64(expected), int64(actual), 11112, "video %d DTS shifted", id)
									if videoFirst {
										require.False(t, sample.IsNonSyncSample, "segment %s", path)
										videoFirst = false
									}
									videos++
								}
								dts += int64(sample.Duration)
							}
						}
					}
				}
				// Each track retains its newest sample as nextSample.
				require.Len(t, seen, count-1)
				require.Equal(t, count-1, videos)
				require.True(t, seenVideo[0], "first session video IDR lost")
				require.True(t, seen[0], "first session audio payload lost")
			})
		}
	}
}

func TestRecorderAudioBoundaryRejectsStalledOrUnknownAbsoluteTime(t *testing.T) {
	ntp := time.Unix(1700000000, 0)
	video := &formatFMP4Track{
		initTrack: &fmp4.InitTrack{
			TimeScale: 90000,
			Codec:     &mcodecs.H264{},
		},
		nextSample: &formatFMP4Sample{
			dts: 90 * 90000,
			ntp: ntp,
		},
	}
	for _, audioNTP := range []time.Time{{}, ntp.Add(-30 * time.Second), ntp.Add(30 * time.Second)} {
		audio := &formatFMP4Track{
			initTrack: &fmp4.InitTrack{
				TimeScale: 8000,
				Codec:     &mcodecs.LPCM{},
			},
			nextSample: &formatFMP4Sample{
				dts: 10 * 8000,
				ntp: audioNTP,
			},
		}
		_, start := nextSegmentStartingPos([]*formatFMP4Track{video, audio}, video)
		require.Equal(t, 90*time.Second, start, "stale audio must not pin segment start")
	}
}
