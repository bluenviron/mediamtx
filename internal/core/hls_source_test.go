package core

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/aler9/gortsplib/v2"
	"github.com/aler9/gortsplib/v2/pkg/codecs/h264"
	"github.com/aler9/gortsplib/v2/pkg/codecs/mpeg4audio"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/media"
	"github.com/aler9/gortsplib/v2/pkg/url"
	"github.com/asticode/go-astits"
	"github.com/gin-gonic/gin"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

type testHLSServer struct {
	s *http.Server

	clientConnected chan struct{}
}

func newTestHLSServer() (*testHLSServer, error) {
	ln, err := net.Listen("tcp", "localhost:5780")
	if err != nil {
		return nil, err
	}

	ts := &testHLSServer{
		clientConnected: make(chan struct{}),
	}

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

	ctx.Writer.Header().Set("Content-Type", `application/x-mpegURL`)
	io.Copy(ctx.Writer, bytes.NewReader([]byte(cnt)))
}

func (ts *testHLSServer) onSegment1(ctx *gin.Context) {
	ctx.Writer.Header().Set("Content-Type", `video/MP2T`)
	mux := astits.NewMuxer(context.Background(), ctx.Writer)

	mux.AddElementaryStream(astits.PMTElementaryStream{
		ElementaryPID: 256,
		StreamType:    astits.StreamTypeH264Video,
	})

	mux.AddElementaryStream(astits.PMTElementaryStream{
		ElementaryPID: 257,
		StreamType:    astits.StreamTypeAACAudio,
	})

	mux.SetPCRPID(256)

	mux.WriteTables()

	enc, _ := h264.AnnexBMarshal([][]byte{
		{1}, // non-IDR
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
				StreamID: 224,
			},
			Data: enc,
		},
	})

	pkts := mpeg4audio.ADTSPackets{
		{
			Type:         2,
			SampleRate:   44100,
			ChannelCount: 2,
			AU:           []byte{0x01, 0x02, 0x03, 0x04},
		},
	}
	enc, _ = pkts.Marshal()

	mux.WriteData(&astits.MuxerData{
		PID: 257,
		PES: &astits.PESData{
			Header: &astits.PESHeader{
				OptionalHeader: &astits.PESOptionalHeader{
					MarkerBits:      2,
					PTSDTSIndicator: astits.PTSDTSIndicatorOnlyPTS,
					PTS:             &astits.ClockReference{Base: int64(1 * 90000)},
				},
				StreamID: 192,
			},
			Data: enc,
		},
	})
}

func (ts *testHLSServer) onSegment2(ctx *gin.Context) {
	<-ts.clientConnected

	ctx.Writer.Header().Set("Content-Type", `video/MP2T`)
	mux := astits.NewMuxer(context.Background(), ctx.Writer)

	mux.AddElementaryStream(astits.PMTElementaryStream{
		ElementaryPID: 256,
		StreamType:    astits.StreamTypeH264Video,
	})

	mux.AddElementaryStream(astits.PMTElementaryStream{
		ElementaryPID: 257,
		StreamType:    astits.StreamTypeAACAudio,
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
					PTS:             &astits.ClockReference{Base: int64(2 * 90000)},
				},
				StreamID: 224, // = video
			},
			Data: enc,
		},
	})

	pkts := mpeg4audio.ADTSPackets{
		{
			Type:         2,
			SampleRate:   44100,
			ChannelCount: 2,
			AU:           []byte{0x01, 0x02, 0x03, 0x04},
		},
	}
	enc, _ = pkts.Marshal()

	mux.WriteData(&astits.MuxerData{
		PID: 257,
		PES: &astits.PESData{
			Header: &astits.PESHeader{
				OptionalHeader: &astits.PESOptionalHeader{
					MarkerBits:      2,
					PTSDTSIndicator: astits.PTSDTSIndicatorOnlyPTS,
					PTS:             &astits.ClockReference{Base: int64(1 * 90000)},
				},
				StreamID: 192,
			},
			Data: enc,
		},
	})

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

	p, ok := newInstance("rtmpDisable: yes\n" +
		"hlsDisable: yes\n" +
		"webrtcDisable: yes\n" +
		"paths:\n" +
		"  proxied:\n" +
		"    source: http://localhost:5780/stream.m3u8\n" +
		"    sourceOnDemand: yes\n")
	require.Equal(t, true, ok)
	defer p.Close()

	frameRecv := make(chan struct{})

	c := gortsplib.Client{}

	u, err := url.Parse("rtsp://localhost:8554/proxied")
	require.NoError(t, err)

	err = c.Start(u.Scheme, u.Host)
	require.NoError(t, err)
	defer c.Close()

	medias, baseURL, _, err := c.Describe(u)
	require.NoError(t, err)

	require.Equal(t, media.Medias{
		{
			Type:    media.TypeVideo,
			Control: medias[0].Control,
			Formats: []format.Format{
				&format.H264{
					PayloadTyp:        96,
					PacketizationMode: 1,
				},
			},
		},
		{
			Type:    media.TypeAudio,
			Control: medias[1].Control,
			Formats: []format.Format{
				&format.MPEG4Audio{
					PayloadTyp: 96,
					Config: &mpeg4audio.Config{
						Type:         2,
						SampleRate:   44100,
						ChannelCount: 2,
					},
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
				},
			},
		},
	}, medias)

	err = c.SetupAll(medias, baseURL)
	require.NoError(t, err)

	c.OnPacketRTP(medias[0], medias[0].Formats[0], func(pkt *rtp.Packet) {
		require.Equal(t, &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: pkt.SequenceNumber,
				Timestamp:      pkt.Timestamp,
				SSRC:           pkt.SSRC,
				CSRC:           []uint32{},
			},
			Payload: []byte{
				0x18,
				0x00, 0x04,
				0x07, 0x01, 0x02, 0x03, // SPS
				0x00, 0x01,
				0x08, // PPS
				0x00, 0x01,
				0x05, // IDR
			},
		}, pkt)
		close(frameRecv)
	})

	_, err = c.Play(nil)
	require.NoError(t, err)

	close(ts.clientConnected)

	<-frameRecv
}
