package core

import (
	"os"
	"path/filepath"
	"strings"
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

// commonConf returns the configuration shared by the subtests of
// TestCoreHotReloadingWaitsForHooks. Every server is switched off, so that no
// port is bound and no certificate loader is created; paths is the content of
// the "paths" section.
func commonConf(logLevel, logFile, paths string) string {
	return "logLevel: " + logLevel + "\n" +
		"logDestinations: [file]\n" +
		"logFile: " + logFile + "\n" +
		"api: no\n" +
		"metrics: no\n" +
		"pprof: no\n" +
		"playback: no\n" +
		"rtsp: no\n" +
		"rtmp: no\n" +
		"hls: no\n" +
		"webrtc: no\n" +
		"srt: no\n" +
		"moq: no\n" +
		"paths:\n" + paths
}

// hookScript returns a shell script that appends "start" to ledger and then
// waits. On SIGINT its trap blocks until release exists, appends "exit" and
// quits, so the ledger records whether the hook finished before a new one was
// started. A hook that never reaches its trap leaves on its own after 1200
// polls of 0.1s and appends "abandoned" instead: that limit is longer than the
// 30s, 3s and 30s this test waits, so a healthy run never reaches it, and far
// below the default go test timeout, so a hook that misses its SIGINT ends the
// test with a failing assertion instead of stalling the whole package. The
// '"'"' sequences close, quote and reopen the single-quoted trap argument,
// which is how a single quote is embedded into a single-quoted word.
func hookScript(ledger, release string) string {
	return "#!/bin/sh\n" +
		`trap 'while [ ! -e '"'"'` + release +
		`'"'"' ]; do sleep 0.05; done; echo exit >> '"'"'` + ledger +
		`'"'"'; exit 0' INT` + "\n" +
		`echo start >> '` + ledger + `'` + "\n" +
		"i=0\n" +
		`while [ "$i" -lt 1200 ]; do sleep 0.1; i=$((i+1)); done` + "\n" +
		`echo abandoned >> '` + ledger + `'` + "\n" +
		"exit 0\n"
}

// waitForFile waits until path exists.
func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, err := os.Stat(path)
		if err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	t.Fatalf("timed out waiting for %s; its directory holds %v (read error %v)", path, names, err)
}

// lineCount returns the number of non-empty lines in path. A file that is not
// there yet counts as zero lines, which is a normal state while polling.
func lineCount(path string) int {
	byts, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	count := 0
	for line := range strings.SplitSeq(string(byts), "\n") {
		if line != "" {
			count++
		}
	}
	return count
}

// readLines returns the non-empty lines of path.
func readLines(t *testing.T, path string) []string {
	t.Helper()

	byts, err := os.ReadFile(path)
	require.NoError(t, err)

	var lines []string
	for line := range strings.SplitSeq(string(byts), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// waitForLines waits until path holds at least n non-empty lines.
func waitForLines(t *testing.T, path string, n int, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if lineCount(path) >= n {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	byts, err := os.ReadFile(path)
	t.Fatalf("timed out waiting for %d lines in %s; it holds %q (read error %v)", n, path, byts, err)
}

// waitForLog waits until the content of path contains needle.
func waitForLog(t *testing.T, path, needle string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		byts, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(byts), needle) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	byts, err := os.ReadFile(path)
	t.Fatalf("timed out waiting for %q in %s; it holds %q (read error %v)", needle, path, byts, err)
}

// pollForLines reports whether path reaches n non-empty lines within window.
// It observes something that is not supposed to happen yet, so it never fails
// the test: a second "start" inside the window is still caught by the final
// require.Equal, which is where the contract is enforced.
func pollForLines(path string, n int, window, interval time.Duration) bool {
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		if lineCount(path) >= n {
			return true
		}
		time.Sleep(interval)
	}
	return false
}

// TestCoreHotReloadingWaitsForHooks checks that a reload which recreates the
// logger drains the external command pool first, so that no hook can log
// through a logger that is being replaced.
func TestCoreHotReloadingWaitsForHooks(t *testing.T) {
	t.Run("logger change drains the hook pool", func(t *testing.T) {
		confDir, dataDir := t.TempDir(), t.TempDir()
		require.NotContains(t, dataDir, "'")
		require.NotContains(t, dataDir, "$")

		confPath := filepath.Join(confDir, "mediamtx.yml")
		logA := filepath.Join(dataDir, "log-a.txt")
		logB := filepath.Join(dataDir, "log-b.txt")

		err := os.WriteFile(confPath, []byte(commonConf("info", logA, "  test:\n")), 0o644)
		require.NoError(t, err)

		p, ok := New([]string{confPath})
		require.Equal(t, true, ok)
		defer p.Close()

		waitForFile(t, logA, 10*time.Second)

		err = os.WriteFile(confPath, []byte(commonConf("info", logB, "  test:\n")), 0o644)
		require.NoError(t, err)

		waitForFile(t, logB, 30*time.Second)

		byts, err := os.ReadFile(logA)
		require.NoError(t, err)
		require.Contains(t, string(byts), "waiting for running hooks")
	})

	t.Run("hooks finish before the reload completes", func(t *testing.T) {
		confDir, dataDir := t.TempDir(), t.TempDir()
		require.NotContains(t, dataDir, "'")
		require.NotContains(t, dataDir, "$")

		confPath := filepath.Join(confDir, "mediamtx.yml")
		logFile := filepath.Join(dataDir, "log-a.txt")
		ledger := filepath.Join(dataDir, "ledger")
		release := filepath.Join(dataDir, "release")
		script := filepath.Join(dataDir, "hook.sh")

		err := os.WriteFile(script, []byte(hookScript(ledger, release)), 0o755)
		require.NoError(t, err)

		paths := "  test:\n    runOnInit: sh '" + script + "'\n"

		err = os.WriteFile(confPath, []byte(commonConf("info", logFile, paths)), 0o644)
		require.NoError(t, err)

		p, ok := New([]string{confPath})
		require.Equal(t, true, ok)

		releaseHook := func() error {
			return os.WriteFile(release, nil, 0o644)
		}

		// Registration order is a contract: LIFO runs releaseHook first, so a
		// failing assertion does not leave Close() waiting on a hook that is
		// already in its trap. A hook that never entered the trap does not read
		// the release file at all; it leaves through the time limit in hookScript.
		defer p.Close()
		defer releaseHook() //nolint:errcheck

		waitForLines(t, ledger, 1, 30*time.Second)

		err = os.WriteFile(confPath, []byte(commonConf("debug", logFile, paths)), 0o644)
		require.NoError(t, err)

		if pollForLines(ledger, 2, 3*time.Second, 50*time.Millisecond) {
			t.Logf("a second hook started while the first one was still running")
		}

		require.NoError(t, releaseHook())
		waitForLines(t, ledger, 3, 30*time.Second)
		require.Equal(t, []string{"start", "exit", "start"}, readLines(t, ledger)[:3])
	})

	t.Run("logger unchanged", func(t *testing.T) {
		confDir, dataDir := t.TempDir(), t.TempDir()
		require.NotContains(t, dataDir, "'")
		require.NotContains(t, dataDir, "$")

		confPath := filepath.Join(confDir, "mediamtx.yml")
		logFile := filepath.Join(dataDir, "log-a.txt")

		err := os.WriteFile(confPath, []byte(commonConf("info", logFile, "  test:\n")), 0o644)
		require.NoError(t, err)

		p, ok := New([]string{confPath})
		require.Equal(t, true, ok)
		defer p.Close()

		waitForFile(t, logFile, 10*time.Second)

		err = os.WriteFile(confPath, []byte(commonConf("info", logFile,
			"  test:\n  test2:\n    runOnInit: sh -c 'exit 0'\n")), 0o644)
		require.NoError(t, err)

		waitForLog(t, logFile, "runOnInit command started", 30*time.Second)

		// Shutdown writes the same message into this very file, so the
		// assertion has to run before the deferred Close().
		byts, err := os.ReadFile(logFile)
		require.NoError(t, err)
		require.NotContains(t, string(byts), "waiting for running hooks")
	})
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
