package core

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/bluenviron/gortsplib/v3"
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/bluenviron/gortsplib/v3/pkg/url"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
	"github.com/gin-gonic/gin"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
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

type testHLSManager struct {
	s *http.Server

	clientConnected chan struct{}
}

func newTestHLSManager() (*testHLSManager, error) {
	ln, err := net.Listen("tcp", "localhost:5780")
	if err != nil {
		return nil, err
	}

	ts := &testHLSManager{
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

func (ts *testHLSManager) close() {
	ts.s.Shutdown(context.Background())
}

func (ts *testHLSManager) onPlaylist(ctx *gin.Context) {
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

func (ts *testHLSManager) onSegment1(ctx *gin.Context) {
	ctx.Writer.Header().Set("Content-Type", `video/MP2T`)

	w := mpegts.NewWriter(ctx.Writer, []*mpegts.Track{track1, track2})

	w.WriteMPEG4Audio(track2, 1*90000, [][]byte{{1, 2, 3, 4}}) //nolint:errcheck
}

func (ts *testHLSManager) onSegment2(ctx *gin.Context) {
	<-ts.clientConnected

	ctx.Writer.Header().Set("Content-Type", `video/MP2T`)

	w := mpegts.NewWriter(ctx.Writer, []*mpegts.Track{track1, track2})

	w.WriteH26x(track1, 2*90000, 2*90000, true, [][]byte{ //nolint:errcheck
		{7, 1, 2, 3}, // SPS
		{8},          // PPS
	})

	w.WriteMPEG4Audio(track2, 2*90000, [][]byte{{1, 2, 3, 4}}) //nolint:errcheck

	w.WriteH26x(track1, 2*90000, 2*90000, true, [][]byte{ //nolint:errcheck
		{5}, // IDR
	})
}

func TestHLSSource(t *testing.T) {
	ts, err := newTestHLSManager()
	require.NoError(t, err)
	defer ts.close()

	p, ok := newInstance("rtmp: no\n" +
		"hls: no\n" +
		"webrtc: no\n" +
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
			Formats: []formats.Format{
				&formats.H264{
					PayloadTyp:        96,
					PacketizationMode: 1,
				},
			},
		},
		{
			Type:    media.TypeAudio,
			Control: medias[1].Control,
			Formats: []formats.Format{
				&formats.MPEG4Audio{
					PayloadTyp:     96,
					ProfileLevelID: 1,
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

	var forma *formats.H264
	medi := medias.FindFormat(&forma)

	_, err = c.Setup(medi, baseURL, 0, 0)
	require.NoError(t, err)

	c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
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
