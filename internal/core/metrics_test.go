package core

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v3"
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/rtmp"
)

func TestMetrics(t *testing.T) {
	serverCertFpath, err := writeTempFile(serverCert)
	require.NoError(t, err)
	defer os.Remove(serverCertFpath)

	serverKeyFpath, err := writeTempFile(serverKey)
	require.NoError(t, err)
	defer os.Remove(serverKeyFpath)

	p, ok := newInstance("hlsAlwaysRemux: yes\n" +
		"metrics: yes\n" +
		"webrtcServerCert: " + serverCertFpath + "\n" +
		"webrtcServerKey: " + serverKeyFpath + "\n" +
		"encryption: optional\n" +
		"serverCert: " + serverCertFpath + "\n" +
		"serverKey: " + serverKeyFpath + "\n" +
		"paths:\n" +
		"  all:\n")
	require.Equal(t, true, ok)
	defer p.Close()

	hc := &http.Client{Transport: &http.Transport{}}

	bo := httpPullFile(t, hc, "http://localhost:9998/metrics")

	require.Equal(t, `paths 0
hls_muxers 0
hls_muxers_bytes_sent 0
rtsp_conns 0
rtsp_conns_bytes_received 0
rtsp_conns_bytes_sent 0
rtsp_sessions 0
rtsp_sessions_bytes_received 0
rtsp_sessions_bytes_sent 0
rtsps_conns 0
rtsps_conns_bytes_received 0
rtsps_conns_bytes_sent 0
rtsps_sessions 0
rtsps_sessions_bytes_received 0
rtsps_sessions_bytes_sent 0
rtmp_conns 0
rtmp_conns_bytes_received 0
rtmp_conns_bytes_sent 0
webrtc_sessions 0
webrtc_sessions_bytes_received 0
webrtc_sessions_bytes_sent 0
`, string(bo))

	medi := testMediaH264

	source := gortsplib.Client{}
	err = source.StartRecording("rtsp://localhost:8554/rtsp_path",
		media.Medias{medi})
	require.NoError(t, err)
	defer source.Close()

	source2 := gortsplib.Client{TLSConfig: &tls.Config{InsecureSkipVerify: true}}
	err = source2.StartRecording("rtsps://localhost:8322/rtsps_path",
		media.Medias{medi})
	require.NoError(t, err)
	defer source2.Close()

	u, err := url.Parse("rtmp://localhost:1935/rtmp_path")
	require.NoError(t, err)

	nconn, err := net.Dial("tcp", u.Host)
	require.NoError(t, err)
	defer nconn.Close()

	conn, err := rtmp.NewClientConn(nconn, u, true)
	require.NoError(t, err)

	videoTrack := &formats.H264{
		PayloadTyp: 96,
		SPS: []byte{ // 1920x1080 baseline
			0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
			0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
			0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
		},
		PPS:               []byte{0x08, 0x06, 0x07, 0x08},
		PacketizationMode: 1,
	}

	_, err = rtmp.NewWriter(conn, videoTrack, nil)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	bo = httpPullFile(t, hc, "http://localhost:9998/metrics")

	require.Regexp(t,
		`^paths\{name=".*?",state="ready"\} 1`+"\n"+
			`paths_bytes_received\{name=".*?",state="ready"\} 0`+"\n"+
			`paths\{name=".*?",state="ready"\} 1`+"\n"+
			`paths_bytes_received\{name=".*?",state="ready"\} 0`+"\n"+
			`paths\{name=".*?",state="ready"\} 1`+"\n"+
			`paths_bytes_received\{name=".*?",state="ready"\} 0`+"\n"+
			`hls_muxers\{name=".*?"\} 1`+"\n"+
			`hls_muxers_bytes_sent\{name=".*?"\} [0-9]+`+"\n"+
			`hls_muxers\{name=".*?"\} 1`+"\n"+
			`hls_muxers_bytes_sent\{name=".*?"\} [0-9]+`+"\n"+
			`hls_muxers\{name=".*?"\} 1`+"\n"+
			`hls_muxers_bytes_sent\{name=".*?"\} [0-9]+`+"\n"+
			`rtsp_conns\{id=".*?"\} 1`+"\n"+
			`rtsp_conns_bytes_received\{id=".*?"\} [0-9]+`+"\n"+
			`rtsp_conns_bytes_sent\{id=".*?"\} [0-9]+`+"\n"+
			`rtsp_sessions\{id=".*?",state="publish"\} 1`+"\n"+
			`rtsp_sessions_bytes_received\{id=".*?",state="publish"\} 0`+"\n"+
			`rtsp_sessions_bytes_sent\{id=".*?",state="publish"\} [0-9]+`+"\n"+
			`rtsps_conns\{id=".*?"\} 1`+"\n"+
			`rtsps_conns_bytes_received\{id=".*?"\} [0-9]+`+"\n"+
			`rtsps_conns_bytes_sent\{id=".*?"\} [0-9]+`+"\n"+
			`rtsps_sessions\{id=".*?",state="publish"\} 1`+"\n"+
			`rtsps_sessions_bytes_received\{id=".*?",state="publish"\} 0`+"\n"+
			`rtsps_sessions_bytes_sent\{id=".*?",state="publish"\} [0-9]+`+"\n"+
			`rtmp_conns\{id=".*?",state="publish"\} 1`+"\n"+
			`rtmp_conns_bytes_received\{id=".*?",state="publish"\} [0-9]+`+"\n"+
			`rtmp_conns_bytes_sent\{id=".*?",state="publish"\} [0-9]+`+"\n"+
			`webrtc_sessions 0`+"\n"+
			`webrtc_sessions_bytes_received 0`+"\n"+
			`webrtc_sessions_bytes_sent 0`+"\n"+
			"$",
		string(bo))
}
