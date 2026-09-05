package core

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/test"
)

func newInstance(t *testing.T, conf string, args ...string) (*Core, bool) {
	if conf == "" {
		return New(args)
	}

	tmpf := test.CreateTempFile(t, []byte(conf))
	args = append(append([]string{}, args...), tmpf)

	return New(args)
}

func srtlaLinkerIsNil(server any) bool {
	serverValue := reflect.ValueOf(server).Elem()
	linker := serverValue.FieldByName("srtlaLinker")
	if !linker.IsValid() {
		linker = serverValue.FieldByName("SRTLALinker")
	}
	return linker.IsNil()
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

func TestCoreHotReloadingSRTLA(t *testing.T) {
	p, ok := newInstance(t, "rtsp: false\n"+
		"rtmp: false\n"+
		"hls: false\n"+
		"webrtc: false\n"+
		"moq: false\n"+
		"srtAddress: 127.0.0.1:0\n"+
		"srtlaAddress: 127.0.0.1:0\n")
	require.True(t, ok)
	defer p.Close()

	srtServer := p.srtServer
	initialSRTLAServer := p.srtlaServer
	require.NotNil(t, srtServer)
	require.NotNil(t, initialSRTLAServer)
	require.False(t, srtlaLinkerIsNil(srtServer))

	newConf := p.conf.Load().Clone()
	newConf.SRTLA = false
	require.NoError(t, newConf.Validate(nil))
	require.NoError(t, p.reloadConf(newConf))
	require.Same(t, srtServer, p.srtServer)
	require.Nil(t, p.srtlaServer)
	require.True(t, srtlaLinkerIsNil(srtServer))

	newConf = p.conf.Load().Clone()
	newConf.SRTLA = true
	require.NoError(t, newConf.Validate(nil))
	require.NoError(t, p.reloadConf(newConf))
	require.Same(t, srtServer, p.srtServer)
	require.NotNil(t, p.srtlaServer)
	require.NotSame(t, initialSRTLAServer, p.srtlaServer)
	require.False(t, srtlaLinkerIsNil(srtServer))
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
