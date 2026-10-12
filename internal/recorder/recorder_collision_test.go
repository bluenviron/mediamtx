package recorder

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/recordstore"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestRecorderSegmentFilenameCollision(t *testing.T) {
	for _, ca := range []struct {
		name   string
		format conf.RecordFormat
	}{
		{"fmp4", conf.RecordFormatFMP4},
		{"mpegts", conf.RecordFormatMPEGTS},
	} {
		t.Run(ca.name, func(t *testing.T) {
			start := time.Date(2026, 1, 2, 12, 0, 0, 123456000, time.Local)
			pathConf := &conf.Path{
				Name:         "camera",
				RecordPath:   filepath.Join(t.TempDir(), "camera", "%Y-%m-%d_%H-%M-%S-%f"),
				RecordFormat: ca.format,
			}
			pathFormat := recordstore.PathAddExtension(pathConf.RecordPath, ca.format)
			var created, completed []string
			onCreate := func(path string) {
				require.FileExists(t, path)
				created = append(created, path)
			}
			onComplete := func(path string, duration time.Duration) {
				require.Equal(t, time.Second, duration)
				completed = append(completed, path)
			}
			originals := make(map[string][]byte)
			starts := make(map[string]time.Time)

			// After progressing normally, the wall clock returns to a previously
			// recorded instant twice. All four segments must survive.
			for i, ntp := range []time.Time{start, start.Add(time.Second), start, start} {
				dts := time.Duration(i) * time.Second
				payload := []byte{0xf8, byte(i), 0xff}
				streamID := uuid.New()
				if ca.format == conf.RecordFormatFMP4 {
					f := &formatFMP4{ri: &recorderInstance{
						streamID:          streamID,
						pathFormat2:       pathFormat,
						partDuration:      time.Second,
						maxPartSize:       1 << 20,
						onSegmentCreate:   onCreate,
						onSegmentComplete: onComplete,
						parent:            test.NilLogger,
					}}
					track := &formatFMP4Track{
						f: f, id: 1, clockRate: 48000, codec: &mcodecs.Opus{ChannelCount: 2},
					}
					track.initialize()
					f.tracks = []*formatFMP4Track{track}
					segment := &formatFMP4Segment{f: f, startDTS: dts, startNTP: ntp, number: uint64(i)}
					segment.initialize()
					require.NoError(t, segment.write(track, &formatFMP4Sample{
						Sample: &fmp4.Sample{Duration: 48000, Payload: payload},
						dts:    int64(i) * 48000,
						ntp:    ntp,
					}, dts))
					require.NoError(t, segment.close())
				} else {
					segment := &formatMPEGTSSegment{
						pathFormat2:       pathFormat,
						flush:             func() error { return nil },
						onSegmentCreate:   onCreate,
						onSegmentComplete: onComplete,
						startDTS:          dts,
						startNTP:          ntp,
						log:               test.NilLogger,
					}
					segment.initialize()
					// A distinct MPEG-TS null packet makes byte preservation observable.
					payload = bytes.Repeat([]byte{byte(i)}, 188)
					copy(payload, []byte{0x47, 0x1f, 0xff, 0x10})
					n, err := segment.Write(payload)
					require.NoError(t, err)
					require.Equal(t, len(payload), n)
					segment.lastDTS = dts + time.Second
					require.NoError(t, segment.close())
				}

				require.Len(t, completed, i+1)
				require.Equal(t, created, completed)
				for path, original := range originals {
					current, err := os.ReadFile(path)
					require.NoError(t, err)
					require.True(t, bytes.Equal(original, current), "previous segment %s was overwritten", path)
				}
				path := completed[i]
				require.NotContains(t, originals, path)
				b, err := os.ReadFile(path)
				require.NoError(t, err)
				originals[path] = b
				starts[path] = ntp
				if ca.format == conf.RecordFormatFMP4 {
					var init fmp4.Init
					require.NoError(t, init.Unmarshal(bytes.NewReader(b)))
					meta := init.UserData[0].(*recordstore.Mtxi)
					require.Equal(t, [16]byte(streamID), meta.StreamID)
					require.Equal(t, uint64(i), meta.SegmentNumber)
					require.Equal(t, int64(dts), meta.DTS)
					require.Equal(t, ntp.UnixNano(), meta.NTP)
					var parts fmp4.Parts
					require.NoError(t, parts.Unmarshal(b))
					require.Len(t, parts, 1)
					require.Len(t, parts[0].Tracks, 1)
					require.Zero(t, parts[0].Tracks[0].BaseTime)
					require.Len(t, parts[0].Tracks[0].Samples, 1)
					require.Equal(t, payload, parts[0].Tracks[0].Samples[0].Payload)
					require.Equal(t, uint32(48000), parts[0].Tracks[0].Samples[0].Duration)
				} else {
					require.Equal(t, payload, b)
				}
			}

			segments, err := recordstore.FindSegments(pathConf, pathConf.Name, nil, nil)
			require.NoError(t, err)
			require.Len(t, segments, len(originals))
			var found []string
			for _, segment := range segments {
				found = append(found, segment.Fpath)
				require.True(t, starts[segment.Fpath].Equal(segment.Start))
			}
			require.ElementsMatch(t, completed, found)
		})
	}
}
