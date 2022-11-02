package core

import (
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
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

	vals := make(map[string]string)
	lines := strings.Split(string(bo), "\n")
	for _, l := range lines[:len(lines)-1] {
		fields := strings.Split(l, " ")
		vals[fields[0]] = fields[1]
	}

	require.Equal(t, map[string]string{
		"hls_muxers{name=\"rtsp_path\"}":            "1",
		"paths{name=\"rtsp_path\",state=\"ready\"}": "1",
		"paths{name=\"rtmp_path\",state=\"ready\"}": "1",
		"rtmp_conns{state=\"idle\"}":                "0",
		"rtmp_conns{state=\"publish\"}":             "1",
		"rtmp_conns{state=\"read\"}":                "0",
		"rtsp_sessions{state=\"idle\"}":             "0",
		"rtsp_sessions{state=\"publish\"}":          "1",
		"rtsp_sessions{state=\"read\"}":             "0",
		"rtsps_sessions{state=\"idle\"}":            "0",
		"rtsps_sessions{state=\"publish\"}":         "0",
		"rtsps_sessions{state=\"read\"}":            "0",
	}, vals)
}
