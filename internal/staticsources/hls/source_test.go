package hls

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/staticsources/tester"
)

var track1 = &mpegts.Track{
	Codec: &mpegts.CodecH264{},
}

var track2 = &mpegts.Track{
	Codec: &mpegts.CodecMPEG4Audio{
		Config: mpeg4audio.Config{
			Type:         2,
			SampleRate:   44100,
			ChannelCount: 2,
		},
	},
}

type testHLSServer struct {
	s *http.Server
}

func newTestHLSServer() (*testHLSServer, error) {
	ln, err := net.Listen("tcp", "localhost:5780")
	if err != nil {
		return nil, err
	}

	ts := &testHLSServer{}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.GET("/stream.m3u8", ts.onPlaylist)
	router.GET("/segment1.ts", ts.onSegment1)
	router.GET("/segment2.ts", ts.onSegment2)

	ts.s = &http.Server{Handler: router}
	go ts.s.Serve(ln)

	return ts, nil
}

func (ts *testHLSServer) close() {
	ts.s.Shutdown(context.Background())
}

func (ts *testHLSServer) onPlaylist(ctx *gin.Context) {
	cnt := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-ALLOW-CACHE:NO
#EXT-X-TARGETDURATION:2
#EXT-X-MEDIA-SEQUENCE:0
#EXTINF:2,
segment1.ts
#EXTINF:2,
segment2.ts
#EXT-X-ENDLIST
`

	ctx.Writer.Header().Set("Content-Type", `application/vnd.apple.mpegurl`)
	io.Copy(ctx.Writer, bytes.NewReader([]byte(cnt)))
}

func (ts *testHLSServer) onSegment1(ctx *gin.Context) {
	ctx.Writer.Header().Set("Content-Type", `video/MP2T`)

	w := mpegts.NewWriter(ctx.Writer, []*mpegts.Track{track1, track2})

	w.WriteMPEG4Audio(track2, 1*90000, [][]byte{{1, 2, 3, 4}}) //nolint:errcheck
}

func (ts *testHLSServer) onSegment2(ctx *gin.Context) {
	ctx.Writer.Header().Set("Content-Type", `video/MP2T`)

	w := mpegts.NewWriter(ctx.Writer, []*mpegts.Track{track1, track2})

	w.WriteH26x(track1, 2*90000, 2*90000, true, [][]byte{ //nolint:errcheck
		{7, 1, 2, 3}, // SPS
		{8},          // PPS
	})
}

func TestSource(t *testing.T) {
	ts, err := newTestHLSServer()
	require.NoError(t, err)
	defer ts.close()

	te := tester.New(
		func(p defs.StaticSourceParent) defs.StaticSource {
			return &Source{
				Parent: p,
			}
		},
		&conf.Path{
			Source: "http://localhost:5780/stream.m3u8",
		},
	)
	defer te.Close()

	<-te.Unit
}
