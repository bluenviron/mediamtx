package core

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v3"
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

type testHTTPAuthenticator struct {
	*http.Server
}

func newTestHTTPAuthenticator(t *testing.T, protocol string, action string) *testHTTPAuthenticator {
	firstReceived := false

	ts := &testHTTPAuthenticator{}

	ts.Server = &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "/auth", r.URL.Path)

			var in struct {
				IP       string `json:"ip"`
				User     string `json:"user"`
				Password string `json:"password"`
				Path     string `json:"path"`
				Protocol string `json:"protocol"`
				ID       string `json:"id"`
				Action   string `json:"action"`
				Query    string `json:"query"`
			}
			err := json.NewDecoder(r.Body).Decode(&in)
			require.NoError(t, err)

			var user string
			if action == "publish" {
				user = "testpublisher"
			} else {
				user = "testreader"
			}

			if in.IP != "127.0.0.1" ||
				in.User != user ||
				in.Password != "testpass" ||
				in.Path != "teststream" ||
				in.Protocol != protocol ||
				(firstReceived && in.ID == "") ||
				in.Action != action ||
				(in.Query != "user=testreader&pass=testpass&param=value" &&
					in.Query != "user=testpublisher&pass=testpass&param=value" &&
					in.Query != "param=value") {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			firstReceived = true
		}),
	}

	ln, err := net.Listen("tcp", "127.0.0.1:9120")
	require.NoError(t, err)

	go ts.Server.Serve(ln)

	return ts
}

func (ts *testHTTPAuthenticator) close() {
	ts.Server.Shutdown(context.Background())
}

func httpPullFile(t *testing.T, hc *http.Client, u string) []byte {
	res, err := hc.Get(u)
	require.NoError(t, err)
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("bad status code: %v", res.StatusCode)
	}

	byts, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	return byts
}

func TestHLSReadNotFound(t *testing.T) {
	p, ok := newInstance("")
	require.Equal(t, true, ok)
	defer p.Close()

	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:8888/stream/", nil)
	require.NoError(t, err)

	hc := &http.Client{Transport: &http.Transport{}}

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestHLSRead(t *testing.T) {
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
		err = source.WritePacketRTP(medi, &rtp.Packet{
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
		require.NoError(t, err)
	}

	hc := &http.Client{Transport: &http.Transport{}}

	cnt := httpPullFile(t, hc, "http://localhost:8888/stream/index.m3u8")
	require.Equal(t, "#EXTM3U\n"+
		"#EXT-X-VERSION:9\n"+
		"#EXT-X-INDEPENDENT-SEGMENTS\n"+
		"\n"+
		"#EXT-X-STREAM-INF:BANDWIDTH=1192,AVERAGE-BANDWIDTH=1192,"+
		"CODECS=\"avc1.42c028\",RESOLUTION=1920x1080,FRAME-RATE=30.000\n"+
		"stream.m3u8\n", string(cnt))

	cnt = httpPullFile(t, hc, "http://localhost:8888/stream/stream.m3u8")
	require.Regexp(t, "#EXTM3U\n"+
		"#EXT-X-VERSION:9\n"+
		"#EXT-X-TARGETDURATION:1\n"+
		"#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES,PART-HOLD-BACK=2\\.50000,CAN-SKIP-UNTIL=6\\.00000\n"+
		"#EXT-X-PART-INF:PART-TARGET=1\\.00000\n"+
		"#EXT-X-MEDIA-SEQUENCE:1\n"+
		"#EXT-X-MAP:URI=\".*?_init.mp4\"\n"+
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
		"#EXT-X-PART:DURATION=1\\.00000,URI=\".*?_part0.mp4\",INDEPENDENT=YES\n"+
		"#EXTINF:1\\.00000,\n"+
		".*?_seg7.mp4\n"+
		"#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\".*?_part1.mp4\"\n", string(cnt))

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
