package core

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
	"github.com/aler9/gortsplib/pkg/url"
	"github.com/asticode/go-astits"
	"github.com/gin-gonic/gin"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
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
#EXT-X-ENDLIST
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

	enc, _ := h264.AnnexBMarshal([][]byte{
		{7, 1, 2, 3}, // SPS
		{8},          // PPS
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

	ctx.Writer.(http.Flusher).Flush()

	time.Sleep(1 * time.Second)

	enc, _ = h264.AnnexBMarshal([][]byte{
		{5}, // IDR
	})

	mux.WriteData(&astits.MuxerData{
		PID: 256,
		PES: &astits.PESData{
			Header: &astits.PESHeader{
				OptionalHeader: &astits.PESOptionalHeader{
					MarkerBits:      2,
					PTSDTSIndicator: astits.PTSDTSIndicatorOnlyPTS,
					PTS:             &astits.ClockReference{Base: int64(2 * 90000)},
				},
				StreamID: 224, // = video
			},
			Data: enc,
		},
	})
}

func TestHLSSource(t *testing.T) {
	ts, err := newTestHLSServer()
	require.NoError(t, err)
	defer ts.close()

	p, ok := newInstance("hlsDisable: yes\n" +
		"rtmpDisable: yes\n" +
		"paths:\n" +
		"  proxied:\n" +
		"    source: http://localhost:5780/stream.m3u8\n" +
		"    sourceOnDemand: yes\n")
	require.Equal(t, true, ok)
	defer p.Close()

	time.Sleep(1 * time.Second)

	frameRecv := make(chan struct{})

	c := gortsplib.Client{
		OnPacketRTP: func(ctx *gortsplib.ClientOnPacketRTPCtx) {
			require.Equal(t, &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					PayloadType:    96,
					SequenceNumber: ctx.Packet.SequenceNumber,
					Timestamp:      ctx.Packet.Timestamp,
					SSRC:           ctx.Packet.SSRC,
					CSRC:           []uint32{},
				},
				Payload: []byte{
					0x18,
					0x00, 0x04,
					0x07, 0x01, 0x02, 0x03, // SPS
					0x00, 0x01,
					0x08, // PPS
					0x00, 0x01,
					0x05, // ODR
				},
			}, ctx.Packet)
			close(frameRecv)
		},
	}

	u, err := url.Parse("rtsp://localhost:8554/proxied")
	require.NoError(t, err)

	err = c.Start(u.Scheme, u.Host)
	require.NoError(t, err)
	defer c.Close()

	tracks, baseURL, _, err := c.Describe(u)
	require.NoError(t, err)

	err = c.SetupAndPlay(tracks, baseURL)
	require.NoError(t, err)

	<-frameRecv
}
