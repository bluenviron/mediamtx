package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v3"
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/gin-gonic/gin"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

type testHTTPAuthenticator struct {
	protocol string
	action   string

	s *http.Server
}

func newTestHTTPAuthenticator(protocol string, action string) (*testHTTPAuthenticator, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:9120")
	if err != nil {
		return nil, err
	}

	ts := &testHTTPAuthenticator{
		protocol: protocol,
		action:   action,
	}

	router := gin.New()
	router.POST("/auth", ts.onAuth)

	ts.s = &http.Server{Handler: router}
	go ts.s.Serve(ln)

	return ts, nil
}

func (ts *testHTTPAuthenticator) close() {
	ts.s.Shutdown(context.Background())
}

func (ts *testHTTPAuthenticator) onAuth(ctx *gin.Context) {
	var in struct {
		IP       string `json:"ip"`
		User     string `json:"user"`
		Password string `json:"password"`
		Path     string `json:"path"`
		Protocol string `json:"protocol"`
		Action   string `json:"action"`
		Query    string `json:"query"`
	}
	err := json.NewDecoder(ctx.Request.Body).Decode(&in)
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	var user string
	if ts.action == "publish" {
		user = "testpublisher"
	} else {
		user = "testreader"
	}

	if in.IP != "127.0.0.1" ||
		in.User != user ||
		in.Password != "testpass" ||
		in.Path != "teststream" ||
		in.Protocol != ts.protocol ||
		in.Action != ts.action ||
		(in.Query != "user=testreader&pass=testpass&param=value" &&
			in.Query != "user=testpublisher&pass=testpass&param=value" &&
			in.Query != "param=value") {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
}

func httpPullFile(u string) ([]byte, error) {
	res, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %v", res.StatusCode)
	}

	return io.ReadAll(res.Body)
}

func TestHLSServerNotFound(t *testing.T) {
	p, ok := newInstance("")
	require.Equal(t, true, ok)
	defer p.Close()

	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:8888/stream/", nil)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestHLSServer(t *testing.T) {
	p, ok := newInstance("hlsAlwaysRemux: yes\n" +
		"paths:\n" +
		"  all:\n")
	require.Equal(t, true, ok)
	defer p.Close()

	medi := &media.Media{
		Type: media.TypeVideo,
		Formats: []formats.Format{&formats.H264{
			PayloadTyp:        96,
			PacketizationMode: 1,
			SPS: []byte{ // 1920x1080 baseline
				0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
				0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
				0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
			},
			PPS: []byte{0x08, 0x06, 0x07, 0x08},
		}},
	}

	v := gortsplib.TransportTCP
	source := gortsplib.Client{
		Transport: &v,
	}
	err := source.StartRecording("rtsp://localhost:8554/stream", media.Medias{medi})
	require.NoError(t, err)
	defer source.Close()

	time.Sleep(500 * time.Millisecond)

	for i := 0; i < 2; i++ {
		source.WritePacketRTP(medi, &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: 123 + uint16(i),
				Timestamp:      45343 + uint32(i*90000),
				SSRC:           563423,
			},
			Payload: []byte{
				0x05, 0x02, 0x03, 0x04, // IDR
			},
		})
	}

	cnt, err := httpPullFile("http://localhost:8888/stream/index.m3u8")
	require.NoError(t, err)
	require.Equal(t, "#EXTM3U\n"+
		"#EXT-X-VERSION:9\n"+
		"#EXT-X-INDEPENDENT-SEGMENTS\n"+
		"\n"+
		"#EXT-X-STREAM-INF:BANDWIDTH=1256,AVERAGE-BANDWIDTH=1256,"+
		"CODECS=\"avc1.42c028\",RESOLUTION=1920x1080,FRAME-RATE=30.000\n"+
		"stream.m3u8\n", string(cnt))

	cnt, err = httpPullFile("http://localhost:8888/stream/stream.m3u8")
	require.NoError(t, err)
	require.Regexp(t, "#EXTM3U\n"+
		"#EXT-X-VERSION:9\n"+
		"#EXT-X-TARGETDURATION:1\n"+
		"#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=2.50000,CAN-SKIP-UNTIL=6\n"+
		"#EXT-X-PART-INF:PART-TARGET=1\n"+
		"#EXT-X-MEDIA-SEQUENCE:1\n"+
		"#EXT-X-MAP:URI=\"init.mp4\"\n"+
		"#EXT-X-GAP\n"+
		"#EXTINF:1\\.00000,\n"+
		"gap.mp4\n"+
		"#EXT-X-GAP\n"+
		"#EXTINF:1\\.00000,\n"+
		"gap.mp4\n"+
		"#EXT-X-GAP\n"+
		"#EXTINF:1\\.00000,\n"+
		"gap.mp4\n"+
		"#EXT-X-GAP\n"+
		"#EXTINF:1\\.00000,\n"+
		"gap.mp4\n"+
		"#EXT-X-GAP\n"+
		"#EXTINF:1\\.00000,\n"+
		"gap.mp4\n"+
		"#EXT-X-GAP\n"+
		"#EXTINF:1\\.00000,\n"+
		"gap.mp4\n"+
		"#EXT-X-PROGRAM-DATE-TIME:.+?Z\n"+
		"#EXT-X-PART:DURATION=1\\.00000,URI=\"part0.mp4\",INDEPENDENT=YES\n"+
		"#EXTINF:1\\.00000,\n"+
		"seg7.mp4\n"+
		"#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"part1.mp4\"\n", string(cnt))

	/*trak := <-c.track

	pkt, _, err := trak.ReadRTP()
	require.NoError(t, err)
	require.Equal(t, &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         true,
			PayloadType:    102,
			SequenceNumber: pkt.SequenceNumber,
			Timestamp:      pkt.Timestamp,
			SSRC:           pkt.SSRC,
			CSRC:           []uint32{},
		},
		Payload: []byte{0x01, 0x02, 0x03, 0x04},
	}, pkt)*/
}
