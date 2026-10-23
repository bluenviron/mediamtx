package core

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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

func TestCoreHotReloadingLogHookOutput(t *testing.T) {
	confPath := filepath.Join(t.TempDir(), "rtsp-conf")

	// all servers are disabled, since the test only needs the configuration
	// reload and the pool flag.
	//
	// the enabled value is the one loaded at startup, so that the initial
	// assertion cannot be satisfied by the zero value of the flag.
	err := os.WriteFile(confPath, []byte("logHookOutput: yes\n"+
		"rtsp: no\n"+
		"rtmp: no\n"+
		"hls: no\n"+
		"webrtc: no\n"+
		"srt: no\n"+
		"moq: no\n"),
		0o644)
	require.NoError(t, err)

	p, ok := New([]string{confPath})
	require.Equal(t, true, ok)
	defer p.Close()

	require.Equal(t, true, p.externalCmdPool.LogOutput.Load())

	err = os.WriteFile(confPath, []byte("logHookOutput: no\n"+
		"rtsp: no\n"+
		"rtmp: no\n"+
		"hls: no\n"+
		"webrtc: no\n"+
		"srt: no\n"+
		"moq: no\n"),
		0o644)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return !p.externalCmdPool.LogOutput.Load()
	}, 5*time.Second, 100*time.Millisecond)
}

// hookOutputConf returns a configuration that writes the log to logFile and
// disables all servers, since the tests of hook output only need hooks and
// configuration reloads.
func hookOutputConf(logFile string) string {
	return "logDestinations: [file]\n" +
		"logFile: " + logFile + "\n" +
		"rtsp: no\n" +
		"rtmp: no\n" +
		"hls: no\n" +
		"webrtc: no\n" +
		"srt: no\n" +
		"moq: no\n"
}

func fileContent(pa string) string {
	byts, _ := os.ReadFile(pa)
	return string(byts)
}

// closeCore closes p, failing the test instead of blocking forever when a hook
// does not exit.
func closeCore(t *testing.T, p *Core) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		p.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout")
	}
}

func TestCoreHotReloadingLogHookOutputNewHooks(t *testing.T) {
	// a reload that changes logHookOutput and adds a path applies the new value
	// to the hook of that path. The reload passes the new paths to the path
	// manager before closing the other components, and the closing is kept busy
	// for a while: the API address changes too, and a client is connected to
	// the API.
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log")
	confPath := filepath.Join(dir, "conf.yml")

	stdout, err := os.Create(filepath.Join(dir, "stdout"))
	require.NoError(t, err)
	defer stdout.Close()

	prevStdout := os.Stdout
	os.Stdout = stdout
	defer func() { os.Stdout = prevStdout }()

	writeConf := func(logHookOutput string, apiAddress string, paths string) {
		err2 := os.WriteFile(confPath, []byte(hookOutputConf(logFile)+
			"logHookOutput: "+logHookOutput+"\n"+
			"api: yes\n"+
			"apiAddress: "+apiAddress+"\n"+
			"paths:\n"+
			"  keep:\n"+
			paths),
			0o644)
		require.NoError(t, err2)
	}

	connectAPI := func(address string) net.Conn {
		deadline := time.Now().Add(10 * time.Second)
		for {
			conn, err2 := net.Dial("tcp", address)
			if err2 == nil {
				return conn
			}
			if time.Now().After(deadline) {
				t.Fatal(err2)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	routedHook := "  routed:\n    runOnInit: sh -c 'echo MARK-ON; exit 1'\n"
	inheritedHook := "  inherited:\n    runOnInit: sh -c 'echo MARK-OFF; exit 1'\n"

	writeConf("no", "127.0.0.1:9997", "")

	p, ok := New([]string{confPath})
	require.Equal(t, true, ok)
	defer closeCore(t, p)

	conn := connectAPI("127.0.0.1:9997")
	writeConf("yes", "127.0.0.1:9998", routedHook)

	require.Eventually(t, func() bool {
		return strings.Contains(fileContent(logFile), "[path routed] runOnInit command exited")
	}, 10*time.Second, 10*time.Millisecond)

	// the API listens on the new address once the reload has completed.
	conn.Close()
	conn = connectAPI("127.0.0.1:9998")
	defer conn.Close()

	// the configuration watcher ignores changes within 1 second from the
	// previous one.
	time.Sleep(1100 * time.Millisecond)
	writeConf("no", "127.0.0.1:9997", routedHook+inheritedHook)

	require.Eventually(t, func() bool {
		return strings.Contains(fileContent(logFile), "[path inherited] runOnInit command exited")
	}, 10*time.Second, 10*time.Millisecond)

	require.Contains(t, fileContent(logFile), "[path routed] MARK-ON")
	require.NotContains(t, fileContent(stdout.Name()), "MARK-ON")
	require.NotContains(t, fileContent(logFile), "MARK-OFF")
	require.Contains(t, fileContent(stdout.Name()), "MARK-OFF")
}

// hookLineIDs returns the numbers of the "L <n>" entries that the hook of path
// "test" wrote to the log file pa, in order.
func hookLineIDs(t *testing.T, pa string) []int {
	t.Helper()

	var ids []int

	for line := range strings.SplitSeq(fileContent(pa), "\n") {
		_, after, ok := strings.Cut(line, "[path test] L ")
		if !ok {
			continue
		}

		id, err := strconv.Atoi(after)
		require.NoError(t, err)
		ids = append(ids, id)
	}

	return ids
}

func TestCoreHotReloadingLogFileWhileHookWrites(t *testing.T) {
	// the hook writes 1000 lines, waits until the log file has been changed,
	// then writes 1000 more lines. The change does not wait for the hook, and
	// each line reaches the file that was in use when it was written.
	dir := t.TempDir()
	logA := filepath.Join(dir, "a.log")
	logB := filepath.Join(dir, "b.log")
	marker := filepath.Join(dir, "marker")
	script := filepath.Join(dir, "hook.sh")

	err := os.WriteFile(script, []byte("i=0\n"+
		"while [ $i -lt 1000 ]; do echo \"L $i\"; i=$((i+1)); done\n"+
		"n=0\n"+
		"while [ ! -f "+marker+" ] && [ $n -lt 500 ]; do sleep 0.01; n=$((n+1)); done\n"+
		"while [ $i -lt 2000 ]; do echo \"L $i\"; i=$((i+1)); done\n"+
		"exit 1\n"),
		0o644)
	require.NoError(t, err)

	confPath := filepath.Join(dir, "conf.yml")

	err = os.WriteFile(confPath, []byte(hookOutputConf(logA)+
		"logHookOutput: yes\n"+
		"api: yes\n"+
		"apiAddress: 127.0.0.1:9997\n"+
		"paths:\n"+
		"  test:\n"+
		"    runOnInit: sh "+script+"\n"),
		0o644)
	require.NoError(t, err)

	// the hook stops waiting after 5 seconds anyway, so the test cannot hang
	// even if the marker is never created.
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			err2 := os.WriteFile(marker, nil, 0o644)
			require.NoError(t, err2)
		})
	}

	p, ok := New([]string{confPath})
	require.Equal(t, true, ok)
	defer closeCore(t, p)

	// deferred after closeCore, so that it runs first.
	defer release()

	require.Eventually(t, func() bool {
		return strings.Contains(fileContent(logA), "[path test] L 999\n")
	}, 10*time.Second, 10*time.Millisecond)

	hc := &http.Client{Timeout: 10 * time.Second}

	httpRequest(t, hc, http.MethodPatch, "http://127.0.0.1:9997/v3/config/global/patch", map[string]any{
		"logFile": logB,
	}, nil)

	// a reload that waited for the hook would complete only after the hook
	// stops waiting for the marker.
	require.Eventually(t, func() bool {
		_, err2 := os.Stat(logB)
		return err2 == nil
	}, 3*time.Second, 10*time.Millisecond)

	release()

	require.Eventually(t, func() bool {
		return strings.Contains(fileContent(logB), "[path test] runOnInit command exited")
	}, 10*time.Second, 10*time.Millisecond)

	lineIDs := func(from int, to int) []int {
		var ids []int
		for i := from; i < to; i++ {
			ids = append(ids, i)
		}
		return ids
	}

	require.Equal(t, lineIDs(0, 1000), hookLineIDs(t, logA))
	require.Equal(t, lineIDs(1000, 2000), hookLineIDs(t, logB))
}

func TestCoreCloseLogsHookOutput(t *testing.T) {
	// a hook that writes while the server is shutting down: the log is closed
	// only after the hooks have exited.
	dir := t.TempDir()
	logFile := filepath.Join(dir, "log")
	script := filepath.Join(dir, "hook.sh")

	err := os.WriteFile(script, []byte("trap 'sleep 0.2; echo LAST; exit 0' INT\n"+
		"echo READY\n"+
		"n=0\n"+
		"while [ $n -lt 100 ]; do sleep 0.1; n=$((n+1)); done\n"),
		0o644)
	require.NoError(t, err)

	confPath := filepath.Join(dir, "conf.yml")

	err = os.WriteFile(confPath, []byte(hookOutputConf(logFile)+
		"logHookOutput: yes\n"+
		"paths:\n"+
		"  test:\n"+
		"    runOnInit: sh "+script+"\n"),
		0o644)
	require.NoError(t, err)

	p, ok := New([]string{confPath})
	require.Equal(t, true, ok)

	// the server is closed exactly once, also when the test fails before the
	// explicit close.
	var closeOnce sync.Once
	closeServer := func() {
		closeOnce.Do(func() { closeCore(t, p) })
	}
	defer closeServer()

	require.Eventually(t, func() bool {
		return strings.Contains(fileContent(logFile), "[path test] READY")
	}, 10*time.Second, 10*time.Millisecond)

	closeServer()

	require.Contains(t, fileContent(logFile), "[path test] LAST")
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
