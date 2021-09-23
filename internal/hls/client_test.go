package hls

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
	"github.com/asticode/go-astits"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/aler9/rtsp-simple-server/internal/logger"
)

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
	router.GET("/segment.ts", ts.onSegment)

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
segment.ts
`

	ctx.Writer.Header().Set("Content-Type", `application/x-mpegURL`)
	io.Copy(ctx.Writer, bytes.NewReader([]byte(cnt)))
}

func (ts *testHLSServer) onSegment(ctx *gin.Context) {
	ctx.Writer.Header().Set("Content-Type", `video/MP2T`)
	mux := astits.NewMuxer(context.Background(), ctx.Writer)

	mux.AddElementaryStream(astits.PMTElementaryStream{
		ElementaryPID: 256,
		StreamType:    astits.StreamTypeH264Video,
	})

	mux.SetPCRPID(256)

	mux.WriteTables()

	enc, _ := h264.EncodeAnnexB([][]byte{
		{7, 1, 2, 3}, // SPS
		{8},          // PPS
		{5},          // IDR
	})

	mux.WriteData(&astits.MuxerData{
		PID: 256,
		PES: &astits.PESData{
			Header: &astits.PESHeader{
				OptionalHeader: &astits.PESOptionalHeader{
					MarkerBits:      2,
					PTSDTSIndicator: astits.PTSDTSIndicatorOnlyPTS,
					PTS:             &astits.ClockReference{Base: int64(1 * 90000)},
				},
				StreamID: 224, // = video
			},
			Data: enc,
		},
	})
}

type testClientParent struct{}

func (testClientParent) Log(level logger.Level, format string, args ...interface{}) {}

func TestClient(t *testing.T) {
	ts, err := newTestHLSServer()
	require.NoError(t, err)
	defer ts.close()

	onTracks := func(*gortsplib.Track, *gortsplib.Track) error {
		return nil
	}

	frameRecv := make(chan struct{})

	onFrame := func(isVideo bool, byts []byte) {
		require.Equal(t, true, isVideo)
		require.Equal(t, byte(0x05), byts[12])
		close(frameRecv)
	}

	c := NewClient(
		"http://localhost:5780/stream.m3u8",
		onTracks,
		onFrame,
		testClientParent{},
	)

	<-frameRecv

	c.Close()
	c.Wait()
}
