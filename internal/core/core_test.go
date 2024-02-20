package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func newInstance(conf string) (*Core, bool) {
	if conf == "" {
		return New([]string{})
	}

	tmpf, err := test.CreateTempFile([]byte(conf))
	if err != nil {
		return nil, false
	}
	defer os.Remove(tmpf)

	return New([]string{tmpf})
}

func TestCoreErrors(t *testing.T) {
	for _, ca := range []struct {
		name string
		conf string
	}{
		{
			"logger",
			"logDestinations: [file]\n" +
				"logFile: /nonexisting/nonexist\n",
		},
		{
			"metrics",
			"metrics: yes\n" +
				"metricsAddress: invalid\n",
		},
		{
			"pprof",
			"pprof: yes\n" +
				"pprofAddress: invalid\n",
		},
		{
			"playback",
			"playback: yes\n" +
				"playbackAddress: invalid\n",
		},
		{
			"rtsp",
			"rtspAddress: invalid\n",
		},
		{
			"rtsps",
			"encryption: strict\n" +
				"rtspAddress: invalid\n",
		},
		{
			"rtmp",
			"rtmpAddress: invalid\n",
		},
		{
			"rtmps",
			"rtmpEncryption: strict\n" +
				"rtmpAddress: invalid\n",
		},
		{
			"hls",
			"hlsAddress: invalid\n",
		},
		{
			"webrtc",
			"webrtcAddress: invalid\n",
		},
		{
			"srt",
			"srtAddress: invalid\n",
		},
		{
			"api",
			"api: yes\n" +
				"apiAddress: invalid\n",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			_, ok := newInstance(ca.conf)
			require.Equal(t, false, ok)
		})
	}
}

func TestCoreHotReloading(t *testing.T) {
	confPath := filepath.Join(os.TempDir(), "rtsp-conf")

	err := os.WriteFile(confPath, []byte("paths:\n"+
		"  test1:\n"+
		"    publishUser: myuser\n"+
		"    publishPass: mypass\n"),
		0o644)
	require.NoError(t, err)
	defer os.Remove(confPath)

	p, ok := New([]string{confPath})
	require.Equal(t, true, ok)
	defer p.Close()

	func() {
		c := gortsplib.Client{}
		err = c.StartRecording("rtsp://localhost:8554/test1",
			&description.Session{Medias: []*description.Media{test.UniqueMediaH264()}})
		require.EqualError(t, err, "bad status code: 401 (Unauthorized)")
	}()

	err = os.WriteFile(confPath, []byte("paths:\n"+
		"  test1:\n"),
		0o644)
	require.NoError(t, err)

	time.Sleep(1 * time.Second)

	func() {
		conn := gortsplib.Client{}
		err = conn.StartRecording("rtsp://localhost:8554/test1",
			&description.Session{Medias: []*description.Media{test.UniqueMediaH264()}})
		require.NoError(t, err)
		defer conn.Close()
	}()
}
