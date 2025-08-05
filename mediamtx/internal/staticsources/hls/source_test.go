package hls

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestSource(t *testing.T) {
	track1 := &mpegts.Track{
		Codec: &mpegts.CodecH264{},
	}

	track2 := &mpegts.Track{
		Codec: &mpegts.CodecMPEG4Audio{
			Config: mpeg4audio.AudioSpecificConfig{
				Type:         2,
				SampleRate:   44100,
				ChannelCount: 2,
			},
		},
	}

	tracks := []*mpegts.Track{
		track1,
		track2,
	}

	s := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/stream.m3u8":
				w.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)
				w.Write([]byte("#EXTM3U\n" +
					"#EXT-X-VERSION:3\n" +
					"#EXT-X-ALLOW-CACHE:NO\n" +
					"#EXT-X-TARGETDURATION:2\n" +
					"#EXT-X-MEDIA-SEQUENCE:0\n" +
					"#EXTINF:2,\n" +
					"segment1.ts\n" +
					"#EXTINF:2,\n" +
					"segment2.ts\n" +
					"#EXTINF:2,\n" +
					"segment2.ts\n" +
					"#EXT-X-ENDLIST\n"))

			case r.Method == http.MethodGet && r.URL.Path == "/segment1.ts":
				w.Header().Set("Content-Type", `video/MP2T`)

				w := &mpegts.Writer{W: w, Tracks: tracks}
				err := w.Initialize()
				require.NoError(t, err)

				err = w.WriteMPEG4Audio(track2, 1*90000, [][]byte{{1, 2, 3, 4}})
				require.NoError(t, err)

				err = w.WriteH264(track1, 2*90000, 2*90000, [][]byte{
					{7, 1, 2, 3}, // SPS
					{8},          // PPS
				})
				require.NoError(t, err)

			case r.Method == http.MethodGet && r.URL.Path == "/segment2.ts":
				w.Header().Set("Content-Type", `video/MP2T`)

				w := &mpegts.Writer{W: w, Tracks: tracks}
				err := w.Initialize()
				require.NoError(t, err)

				err = w.WriteMPEG4Audio(track2, 3*90000, [][]byte{{1, 2, 3, 4}})
				require.NoError(t, err)
			}
		}),
	}

	ln, err := net.Listen("tcp", "localhost:5780")
	require.NoError(t, err)

	go s.Serve(ln)
	defer s.Shutdown(context.Background())

	te := test.NewSourceTester(
		func(p defs.StaticSourceParent) defs.StaticSource {
			return &Source{
				Parent: p,
			}
		},
		"http://localhost:5780/stream.m3u8",
		&conf.Path{},
	)
	defer te.Close()

	<-te.Unit
}
