package core

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/logger"
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

func TestCoreLogWithoutLogger(t *testing.T) {
	p := &Core{}

	require.NotPanics(t, func() {
		p.Log(logger.Info, "test %d", 1)
	})
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

func TestCoreHotReloadingWhileLogging(t *testing.T) {
	confPath := filepath.Join(t.TempDir(), "rtsp-conf")

	// all servers are disabled, since the test only needs the configuration
	// reload and Core.Log().
	writeConf := func(logLevel string) {
		err := os.WriteFile(confPath, []byte("logLevel: "+logLevel+"\n"+
			"rtsp: no\n"+
			"rtmp: no\n"+
			"hls: no\n"+
			"webrtc: no\n"+
			"srt: no\n"+
			"moq: no\n"),
			0o644)
		require.NoError(t, err)
	}

	writeConf("error")

	p, ok := New([]string{confPath})
	require.Equal(t, true, ok)
	defer p.Close()

	// call Log() continuously, in order to read the logger while a
	// configuration reload is replacing it. two goroutines are enough to keep
	// a read in flight for the entire duration of the replacement.
	// the level is Debug, that is filtered out by the log levels used by this
	// test, therefore nothing is printed; but the filter is inside
	// logger.Logger.Log(), i.e. after the logger has been read from the Core,
	// therefore the race is triggered anyway.
	stop := make(chan struct{})
	var loggers sync.WaitGroup

	// stop logging before closing the server.
	defer func() {
		close(stop)
		loggers.Wait()
	}()

	for range 2 {
		loggers.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}

				p.Log(logger.Debug, "test")
			}
		})
	}

	// change the log level back and forth: each change replaces the logger
	// and therefore opens a race window. two changes are enough to detect it.
	for _, logLevel := range []string{"warn", "error"} {
		// the configuration watcher discards changes that happen less than
		// one second after the previous reload.
		time.Sleep(1100 * time.Millisecond)

		oldLogger := p.logger.Load()

		writeConf(logLevel)

		// make sure that the logger has been replaced. checking the
		// configuration is not enough, since it is stored before the
		// resources are recreated, therefore it is updated even when the
		// reload fails. a nil logger does not mean that the replacement is
		// done: it is either the window inside a reload or the state that
		// a failed reload leaves behind.
		require.Eventually(t, func() bool {
			l := p.logger.Load()
			return l != nil && l != oldLogger
		}, 3*time.Second, 20*time.Millisecond)
	}
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
