package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/test"
)

func newInstance(t *testing.T, conf string, args ...string) (*Core, bool) {
	t.Helper()

	if conf == "" {
		return New(args)
	}

	tmpf := test.CreateTempFile(t, []byte(conf))
	args = append(append([]string{}, args...), tmpf)

	return New(args)
}

func TestCoreFatalError(t *testing.T) {
	for _, name := range []string{
		"rtsp", "rtsps", "rtmp", "rtmps", "hls", "webrtc", "srt", "moq",
		"api", "metrics", "pprof", "playback",
	} {
		t.Run(name, func(t *testing.T) {
			service := name
			config := name + "Address: 127.0.0.1:0\n"
			switch name {
			case "rtsp":
				config += "rtspTransports: [tcp]\n"
			case "rtsps":
				service = "rtsp"
				config += "rtspEncryption: strict\n"
			case "rtmps":
				service = "rtmp"
				config += "rtmpEncryption: strict\n"
			case "webrtc":
				config += "webrtcLocalUDPAddress: 127.0.0.1:0\n"
			case "moq":
				config = "moqHTTP2Address: 127.0.0.1:0\n" +
					"moqHTTP3Address: 127.0.0.1:0\n" +
					"moqQUICAddress: 127.0.0.1:0\n"
			}
			if name == "rtsps" || name == "rtmps" {
				config += service + "ServerCert: " + test.CreateTempFile(t, test.TLSCertPub) + "\n" +
					service + "ServerKey: " + test.CreateTempFile(t, test.TLSCertKey) + "\n"
			}
			config += service + ": yes\n"
			for _, other := range []string{"rtsp", "rtmp", "hls", "webrtc", "srt", "moq"} {
				if other != service {
					config += other + ": no\n"
				}
			}
			p, ok := newInstance(t, config)
			require.True(t, ok)
			t.Cleanup(p.Close)

			var onFatalError func(context.Context, error)
			switch name {
			case "rtsp":
				onFatalError = p.rtspServer.Parent.OnFatalError
			case "rtsps":
				onFatalError = p.rtspsServer.Parent.OnFatalError
			case "rtmp":
				onFatalError = p.rtmpServer.Parent.OnFatalError
			case "rtmps":
				onFatalError = p.rtmpsServer.Parent.OnFatalError
			case "hls":
				onFatalError = p.hlsServer.Parent.OnFatalError
			case "webrtc":
				onFatalError = p.webRTCServer.Parent.OnFatalError
			case "srt":
				onFatalError = p.srtServer.Parent.OnFatalError
			case "moq":
				onFatalError = p.moqServer.Parent.OnFatalError
			case "api":
				onFatalError = p.api.Parent.OnFatalError
			case "metrics":
				onFatalError = p.metrics.Parent.OnFatalError
			case "pprof":
				onFatalError = p.pprof.Parent.OnFatalError
			case "playback":
				onFatalError = p.playbackServer.Parent.OnFatalError
			}
			require.NotNil(t, onFatalError)
			onFatalError(context.Background(), errors.New("listen failed"))

			select {
			case <-p.done:
			case <-time.After(5 * time.Second):
				t.Fatal("listener error did not shut down core")
			}
		})
	}
}

func TestCoreFatalErrorCanceledSource(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &Core{
		ctx:          context.Background(),
		chFatalError: make(chan struct{}),
	}
	done := make(chan struct{})
	go func() {
		p.OnFatalError(ctx, errors.New("listen failed"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("canceled listener callback blocked")
	}
}

func TestCoreErrors(t *testing.T) {
	for _, ca := range []struct {
		name string
		conf string
	}{
		{
			"logger",
			"logDestinations: [file]\n" +
				"logFile: /nonexisting/nonexist\n" +
				"sysLogPrefix: /mediamtx\n",
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
			"rtspEncryption: strict\n" +
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
			_, ok := newInstance(t, ca.conf)
			require.Equal(t, false, ok)
		})
	}
}

func TestCoreHotReloading(t *testing.T) {
	confPath := filepath.Join(t.TempDir(), "rtsp-conf")

	err := os.WriteFile(confPath, []byte("paths:\n"+
		"  test1:\n"+
		"    publishUser: myuser\n"+
		"    publishPass: mypass\n"),
		0o644)
	require.NoError(t, err)

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

func TestCoreHotReloadingAndLoggerError(t *testing.T) {
	confPath := filepath.Join(t.TempDir(), "rtsp-conf")

	err := os.WriteFile(confPath, []byte(""),
		0o644)
	require.NoError(t, err)

	p, ok := New([]string{confPath})
	require.Equal(t, true, ok)
	defer p.Close()

	err = os.WriteFile(confPath, []byte("logDestinations: [file]\n"+
		"logFile: /nonexisting/nonexist\n"),
		0o644)
	require.NoError(t, err)

	p.Wait()
}

func TestNewRejectsConflictingOneShotFlags(t *testing.T) {
	_, ok := newInstance(t, "", "--version", "--validate-conf=test.yml")
	require.Equal(t, false, ok)
}

func TestValidateConf(t *testing.T) {
	savedDefaultConfPaths := defaultConfPaths
	savedDefaultConfPathsNotWin := defaultConfPathsNotWin
	t.Cleanup(func() {
		defaultConfPaths = savedDefaultConfPaths
		defaultConfPathsNotWin = savedDefaultConfPathsNotWin
	})

	writeTempConf := func(t *testing.T, content string) string {
		t.Helper()

		pa := filepath.Join(t.TempDir(), "mediamtx.yml")
		err := os.WriteFile(pa, []byte(content), 0o644)
		require.NoError(t, err)
		return pa
	}

	t.Run("explicit valid path", func(t *testing.T) {
		defaultConfPaths = nil
		defaultConfPathsNotWin = nil

		confPath := writeTempConf(t, "paths:\n  all_others:\n")

		ok := validateConf(confPath)
		require.Equal(t, true, ok)
	})

	t.Run("explicit invalid path", func(t *testing.T) {
		defaultConfPaths = nil
		defaultConfPathsNotWin = nil

		ok := validateConf(writeTempConf(t, "writeQueueSize: 3\n"))
		require.Equal(t, false, ok)
	})

	t.Run("environment variables are applied", func(t *testing.T) {
		defaultConfPaths = nil
		defaultConfPathsNotWin = nil
		t.Setenv("MTX_WRITEQUEUESIZE", "3")

		ok := validateConf(writeTempConf(t, "paths:\n  all_others:\n"))
		require.Equal(t, false, ok)
	})
}
