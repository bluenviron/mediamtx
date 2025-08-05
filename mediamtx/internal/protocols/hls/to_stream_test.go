package hls

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/stretchr/testify/require"
)

func TestToStreamNoSupportedCodecs(t *testing.T) {
	_, err := ToStream(nil, []*gohlslib.Track{}, nil)
	require.Equal(t, ErrNoSupportedCodecs, err)
}

// this is impossible to test since currently we support all gohlslib.Tracks.
// func TestToStreamSkipUnsupportedTracks(t *testing.T)

func TestToStream(t *testing.T) {
	track1 := &mpegts.Track{
		Codec: &mpegts.CodecH264{},
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
					"#EXT-X-PROGRAM-DATE-TIME:2018-05-20T08:17:15Z\n" +
					"#EXTINF:2,\n" +
					"segment1.ts\n" +
					"#EXTINF:2,\n" +
					"segment2.ts\n" +
					"#EXTINF:2,\n" +
					"segment2.ts\n" +
					"#EXT-X-ENDLIST\n"))

			case r.Method == http.MethodGet && r.URL.Path == "/segment1.ts":
				w.Header().Set("Content-Type", `video/MP2T`)

				w := &mpegts.Writer{W: w, Tracks: []*mpegts.Track{track1}}
				err := w.Initialize()
				require.NoError(t, err)

				err = w.WriteH264(track1, 2*90000, 2*90000, [][]byte{
					{7, 1, 2, 3}, // SPS
					{8},          // PPS
				})
				require.NoError(t, err)

			case r.Method == http.MethodGet && r.URL.Path == "/segment2.ts":
				w.Header().Set("Content-Type", `video/MP2T`)

				w := &mpegts.Writer{W: w, Tracks: []*mpegts.Track{track1}}
				err := w.Initialize()
				require.NoError(t, err)

				err = w.WriteH264(track1, 2*90000, 2*90000, [][]byte{
					{5, 1},
				})
				require.NoError(t, err)
			}
		}),
	}

	ln, err := net.Listen("tcp", "localhost:5781")
	require.NoError(t, err)

	go s.Serve(ln)
	defer s.Shutdown(context.Background())

	var strm *stream.Stream
	done := make(chan struct{})

	reader := test.NilLogger

	var c *gohlslib.Client
	c = &gohlslib.Client{
		URI: "http://localhost:5781/stream.m3u8",
		OnTracks: func(tracks []*gohlslib.Track) error {
			medias, err2 := ToStream(c, tracks, &strm)
			require.NoError(t, err2)
			require.Equal(t, []*description.Media{{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.H264{
					PayloadTyp:        96,
					PacketizationMode: 1,
				}},
			}}, medias)

			strm = &stream.Stream{
				WriteQueueSize:     512,
				RTPMaxPayloadSize:  1450,
				Desc:               &description.Session{Medias: medias},
				GenerateRTPPackets: true,
				Parent:             test.NilLogger,
			}
			err2 = strm.Initialize()
			require.NoError(t, err2)

			strm.AddReader(
				reader,
				medias[0],
				medias[0].Formats[0],
				func(u unit.Unit) error {
					require.Equal(t, time.Date(2018, 0o5, 20, 8, 17, 15, 0, time.UTC), u.GetNTP())
					close(done)
					return nil
				})

			strm.StartReader(reader)

			return nil
		},
	}
	err = c.Start()
	require.NoError(t, err)
	defer c.Close()

	<-done

	strm.RemoveReader(reader)
	strm.Close()
}
