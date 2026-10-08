package recorder

import (
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	rtspformat "github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func TestRecorderTimestampRewind(t *testing.T) {
	for _, ca := range []string{"rewind", "stall", "jitter"} {
		t.Run(ca, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				desc := &description.Session{Medias: []*description.Media{{
					Type: description.MediaTypeAudio,
					Formats: []rtspformat.Format{&rtspformat.Opus{
						PayloadTyp:   96,
						ChannelCount: 2,
					}},
				}}}
				strm := &stream.Stream{
					OrigDesc:          desc,
					ReplaceNTP:        true,
					WriteQueueSize:    512,
					RTPMaxPayloadSize: 1450,
					Parent:            test.NilLogger,
				}
				require.NoError(t, strm.Initialize())
				defer strm.Close()
				sub := &stream.SubStream{Stream: strm}
				require.NoError(t, sub.Initialize())

				var files []string
				var durations []time.Duration
				r := &Recorder{
					PathFormat:      filepath.Join(t.TempDir(), "%Y-%m-%d_%H-%M-%S-%f"),
					Format:          conf.RecordFormatFMP4,
					PartDuration:    time.Second,
					SegmentDuration: 5 * time.Second,
					MaxPartSize:     1 << 20,
					PathName:        "rewind",
					Stream:          strm,
					Parent:          test.NilLogger,
					OnSegmentComplete: func(path string, d time.Duration) {
						files = append(files, path)
						durations = append(durations, d)
					},
				}
				r.Initialize()
				initialInstance := r.currentInstance

				const frames = 1500
				for i := range frames {
					pts := int64(3600*48000 + i*960)
					switch {
					case ca == "rewind" && i >= 100:
						// Reset the source media clock after two seconds. The real
						// NTP estimator continues to supply the local wall clock.
						pts -= 3600 * 48000
					case ca == "stall" && i >= 100:
						// Repeated equal timestamps must also recover after the
						// estimator corrects a media clock that has stopped.
						pts = 3600*48000 + int64(99+max(i-499, 0))*960
					case ca == "jitter" && i == 100:
						// A single slightly reordered sample must not restart recording.
						pts -= 2 * 960
					}
					sub.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
						PTS:     pts,
						Payload: unit.PayloadOpus{{0xf8, 0xff, 0xfe}},
					})
					synctest.Wait()
					time.Sleep(20 * time.Millisecond)
					synctest.Wait()
				}
				r.Close()

				require.NotEmpty(t, durations)
				for _, duration := range durations {
					require.Positive(t, duration)
				}
				if ca != "jitter" {
					require.NotSame(t, initialInstance, r.currentInstance)
				} else {
					require.Same(t, initialInstance, r.currentInstance)
				}

				samples := 0
				for fileIndex, path := range files {
					b, err := os.ReadFile(path)
					require.NoError(t, err)
					var parts fmp4.Parts
					require.NoError(t, parts.Unmarshal(b))
					for _, part := range parts {
						for _, track := range part.Tracks {
							for _, sample := range track.Samples {
								if (ca == "rewind" && fileIndex != 0) ||
									(ca == "stall" && fileIndex == len(files)-1) {
									require.Equal(t, uint32(960), sample.Duration)
								}
								samples++
							}
						}
					}
				}
				if ca != "jitter" {
					// Only the restart pause and lookahead samples may be lost.
					require.GreaterOrEqual(t, samples, frames-105)
				} else {
					require.Equal(t, frames-1, samples)
				}
			})
		})
	}
}
