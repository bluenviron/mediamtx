package core

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/aler9/gortsplib"
	"github.com/stretchr/testify/require"

	"github.com/aler9/rtsp-simple-server/internal/rtmp"
)

func TestMetrics(t *testing.T) {
	serverCertFpath, err := writeTempFile(serverCert)
	require.NoError(t, err)
	defer os.Remove(serverCertFpath)

	serverKeyFpath, err := writeTempFile(serverKey)
	require.NoError(t, err)
	defer os.Remove(serverKeyFpath)

	p, ok := newInstance("metrics: yes\n" +
		"encryption: optional\n" +
		"serverCert: " + serverCertFpath + "\n" +
		"serverKey: " + serverKeyFpath + "\n" +
		"paths:\n" +
		"  all:\n")
	require.Equal(t, true, ok)
	defer p.Close()

	track := &gortsplib.TrackH264{
		PayloadType: 96,
		SPS:         []byte{0x01, 0x02, 0x03, 0x04},
		PPS:         []byte{0x01, 0x02, 0x03, 0x04},
	}

	source := gortsplib.Client{}
	err = source.StartPublishing("rtsp://localhost:8554/rtsp_path",
		gortsplib.Tracks{track})
	require.NoError(t, err)
	defer source.Close()

	source2 := gortsplib.Client{TLSConfig: &tls.Config{InsecureSkipVerify: true}}
	err = source2.StartPublishing("rtsps://localhost:8322/rtsps_path",
		gortsplib.Tracks{track})
	require.NoError(t, err)
	defer source2.Close()

	u, err := url.Parse("rtmp://localhost:1935/rtmp_path")
	require.NoError(t, err)

	nconn, err := net.Dial("tcp", u.Host)
	require.NoError(t, err)
	defer nconn.Close()
	conn := rtmp.NewConn(nconn)

	err = conn.InitializeClient(u, true)
	require.NoError(t, err)

	videoTrack := &gortsplib.TrackH264{
		PayloadType: 96,
		SPS: []byte{ // 1920x1080 baseline
			0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
			0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
			0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
		},
		PPS: []byte{0x08, 0x06, 0x07, 0x08},
	}

	err = conn.WriteTracks(videoTrack, nil)
	require.NoError(t, err)

	func() {
		res, err := http.Get("http://localhost:8888/rtsp_path/index.m3u8")
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, 200, res.StatusCode)
	}()

	req, err := http.NewRequest(http.MethodGet, "http://localhost:9998/metrics", nil)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	bo, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	require.Regexp(t,
		`^paths\{name=".*?",state="ready"\} 1`+"\n"+
			`paths_bytes_received\{name=".*?",state="ready"\} 0`+"\n"+
			`paths\{name=".*?",state="ready"\} 1`+"\n"+
			`paths_bytes_received\{name=".*?",state="ready"\} 0`+"\n"+
			`paths\{name=".*?",state="ready"\} 1`+"\n"+
			`paths_bytes_received\{name=".*?",state="ready"\} 0`+"\n"+
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
			`hls_muxers\{name="rtsp_path"\} 1`+"\n"+
			`hls_muxers_bytes_sent\{name="rtsp_path"\} [0-9]+`+"\n"+"$",
		string(bo))
}
