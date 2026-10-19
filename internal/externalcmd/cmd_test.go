package externalcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/logger"
)

// logRecorder collects log entries. Its Log method is called by the goroutine
// that copies the output of the command, therefore it is synchronized.
// internal/test cannot be used here, since it depends on this package.
type logRecorder struct {
	onEntry func(logger.Level, string)

	mutex   sync.Mutex
	levels  []logger.Level
	entries []string
}

func (r *logRecorder) Log(level logger.Level, format string, args ...any) {
	entry := fmt.Sprintf(format, args...)

	r.mutex.Lock()
	r.levels = append(r.levels, level)
	r.entries = append(r.entries, entry)
	r.mutex.Unlock()

	if r.onEntry != nil {
		r.onEntry(level, entry)
	}
}

func (r *logRecorder) recorded() ([]logger.Level, []string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return append([]logger.Level(nil), r.levels...), append([]string(nil), r.entries...)
}

// closePool waits for the commands of p to exit, failing the test instead of
// blocking forever when one of them does not.
func closePool(t *testing.T, p *Pool) {
	t.Helper()

	poolClosed := make(chan struct{})
	go func() {
		p.Close()
		close(poolClosed)
	}()

	select {
	case <-poolClosed:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout")
	}
}

// orphanCommand returns a command that runs output, leaves behind a
// descendant that inherits the output pipe and keeps it open, and exits with
// exitCode. The descendant is killed when the test ends.
func orphanCommand(t *testing.T, output string, exitCode int) string {
	t.Helper()

	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")
	script := filepath.Join(dir, "script.sh")

	err := os.WriteFile(script, []byte(output+"\n"+
		"sleep 30 &\n"+
		"echo $! > "+pidFile+"\n"+
		"exit "+strconv.Itoa(exitCode)+"\n"),
		0o644)
	require.NoError(t, err)

	t.Cleanup(func() {
		byts, err2 := os.ReadFile(pidFile)
		if err2 != nil {
			return
		}

		pid, err2 := strconv.Atoi(strings.TrimSpace(string(byts)))
		if err2 != nil {
			return
		}

		proc, err2 := os.FindProcess(pid)
		if err2 != nil {
			return
		}

		proc.Kill() //nolint:errcheck
	})

	return "sh " + script
}

func TestCmdRunExpandAfterSplit(t *testing.T) {
	// if os.Expand runs before shellquote.Split, a variable value containing a
	// single quote produces unbalanced quotes that cause shellquote.Split to fail.
	p := &Pool{}
	p.Initialize()

	out := filepath.Join(t.TempDir(), "out")

	cmd := &Cmd{
		Pool:   p,
		Cmdstr: "sh -c 'echo \"$MY_VAR\" > " + out + "'",
		Env: Environment{
			"MY_VAR": "it's",
		},
	}
	cmd.Start()

	poolClosed := make(chan struct{})
	go func() {
		p.Close()
		close(poolClosed)
	}()

	select {
	case <-poolClosed:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout")
	}

	byts, err := os.ReadFile(out)
	require.NoError(t, err)
	require.Equal(t, "it's\n", string(byts))
}

func TestCmdExitCode(t *testing.T) {
	for _, ca := range []struct {
		name    string
		restart bool
	}{
		{"standard", false},
		{"restart", true},
	} {
		t.Run(ca.name, func(t *testing.T) {
			p := &Pool{}
			p.Initialize()
			// Close() only waits on the WaitGroup: cmd.Close() must run first
			// (defers are LIFO), otherwise the restart case never returns.
			defer p.Close()

			exited := make(chan error, 1)

			cmd := &Cmd{
				Pool:    p,
				Cmdstr:  "sh -c 'exit 5'",
				Restart: ca.restart,
				OnExit: func(err error) {
					select {
					case exited <- err:
					default:
					}
				},
			}
			cmd.Start()
			defer cmd.Close()

			select {
			case err := <-exited:
				require.EqualError(t, err, "command exited with code 5")
			case <-time.After(10 * time.Second):
				t.Fatal("timeout")
			}
		})
	}
}

func TestCmdOutputToLogger(t *testing.T) {
	rec := &logRecorder{}
	ol := &outputLogger{dest: rec}

	in := []byte("first line\nsecond line\n")
	n, err := ol.Write(in)
	require.NoError(t, err)
	require.Equal(t, len(in), n)
	ol.close()

	levels, entries := rec.recorded()
	require.Equal(t, []string{"first line", "second line"}, entries)
	require.Equal(t, []logger.Level{logger.Info, logger.Info}, levels)
}

func TestCmdOutputFormatVerb(t *testing.T) {
	rec := &logRecorder{}
	ol := &outputLogger{dest: rec}

	_, err := ol.Write([]byte("100% done %s %d %!\n"))
	require.NoError(t, err)
	ol.close()

	_, entries := rec.recorded()
	require.Equal(t, []string{"100% done %s %d %!"}, entries)
}

func TestCmdOutputPartialLine(t *testing.T) {
	rec := &logRecorder{}
	ol := &outputLogger{dest: rec}

	_, err := ol.Write([]byte("no newline"))
	require.NoError(t, err)
	_, entries := rec.recorded()
	require.Equal(t, []string(nil), entries)

	ol.close()
	_, entries = rec.recorded()
	require.Equal(t, []string{"no newline"}, entries)

	// after close, output is discarded.
	_, err = ol.Write([]byte("late\n"))
	require.NoError(t, err)
	_, entries = rec.recorded()
	require.Equal(t, []string{"no newline"}, entries)
}

func TestCmdOutputPartialLineViaCommand(t *testing.T) {
	// runOSSpecific closes the output logger when the command ends: that is what
	// makes the tail of a command that never emits '\n' reach the logger.
	p := &Pool{}
	p.Initialize()
	p.LogOutput.Store(true)

	rec := &logRecorder{}

	cmd := &Cmd{
		Pool:         p,
		Cmdstr:       "sh -c 'printf tail-no-newline'",
		OutputLogger: rec,
	}
	cmd.Start()
	closePool(t, p)

	_, entries := rec.recorded()
	require.Equal(t, []string{"tail-no-newline"}, entries)
}

func TestCmdOutputCRLF(t *testing.T) {
	rec := &logRecorder{}
	ol := &outputLogger{dest: rec}

	_, err := ol.Write([]byte("windows\r\n\r\nmid\rline\n"))
	require.NoError(t, err)
	ol.close()

	_, entries := rec.recorded()
	require.Equal(t, []string{"windows", "mid\rline"}, entries)
}

func TestCmdOutputLineCap(t *testing.T) {
	rec := &logRecorder{}
	ol := &outputLogger{dest: rec}

	head := strings.Repeat("a", maxOutputLineLength-1) + "\r"
	_, err := ol.Write([]byte(head + "tail\n"))
	require.NoError(t, err)
	ol.close()

	// the record boundary is forced, not caused by '\n': the '\r' is kept.
	_, entries := rec.recorded()
	require.Equal(t, []string{head, "tail"}, entries)
}

func TestCmdOutputLineCapUTF8(t *testing.T) {
	// the boundary never falls inside a multi-byte sequence, also when the
	// sequence arrives split across two writes; invalid bytes are not treated
	// specially.
	split := func(t *testing.T, chunks ...string) []string {
		t.Helper()

		rec := &logRecorder{}
		ol := &outputLogger{dest: rec}

		for _, chunk := range chunks {
			_, err := ol.Write([]byte(chunk))
			require.NoError(t, err)
		}
		ol.close()

		_, entries := rec.recorded()
		return entries
	}

	head := strings.Repeat("a", maxOutputLineLength-1)
	full := strings.Repeat("a", maxOutputLineLength)

	for _, ca := range []struct {
		name    string
		chunks  []string
		entries []string
		valid   bool
	}{
		{"single-write", []string{head + "한\n"}, []string{head, "한"}, true},
		{"write-boundary", []string{head + "\xed", "\x95\x9c\n"}, []string{head, "한"}, true},
		{"between-chars", []string{full + "b\n"}, []string{full, "b"}, false},
		{"invalid-lone-cont", []string{head + "\x95", "b\n"}, []string{head + "\x95", "b"}, false},
	} {
		t.Run(ca.name, func(t *testing.T) {
			entries := split(t, ca.chunks...)
			require.Equal(t, ca.entries, entries)

			if ca.valid {
				for _, entry := range entries {
					require.True(t, utf8.ValidString(entry))
				}
			}
		})
	}
}

func TestCmdOutputDisabled(t *testing.T) {
	// when disabled, the command keeps inheriting the standard streams of the
	// server, and an OutputLogger that happens to be set is not used at all.
	// A typed nil reaching cmd.Stdout would make os/exec create a pipe and
	// panic inside its copying goroutine, taking down the test binary.
	out := filepath.Join(t.TempDir(), "out")
	f, err := os.Create(out)
	require.NoError(t, err)
	defer f.Close()

	prevStdout := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = prevStdout }()

	p := &Pool{}
	p.Initialize()

	rec := &logRecorder{}

	cmd := &Cmd{
		Pool:         p,
		Cmdstr:       "sh -c 'echo inherited'",
		OutputLogger: rec,
	}
	cmd.Start()
	closePool(t, p)

	byts, err := os.ReadFile(out)
	require.NoError(t, err)
	require.Equal(t, "inherited\n", string(byts))

	_, entries := rec.recorded()
	require.Equal(t, []string(nil), entries)
}

func TestCmdOutputRuntimeToggle(t *testing.T) {
	// LogOutput is read when a command starts.
	// the first command inherits the standard streams, which are redirected to a
	// file so that its output does not end up in the output of the test.
	f, err := os.Create(filepath.Join(t.TempDir(), "out"))
	require.NoError(t, err)
	defer f.Close()

	prevStdout := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = prevStdout }()

	p := &Pool{}
	p.Initialize()

	rec := &logRecorder{}

	newCmd := func() *Cmd {
		return &Cmd{
			Pool:         p,
			Cmdstr:       "sh -c 'echo hello'",
			OutputLogger: rec,
		}
	}

	newCmd().Start()
	closePool(t, p)

	_, entries := rec.recorded()
	require.Equal(t, []string(nil), entries)

	p.LogOutput.Store(true)

	newCmd().Start()
	closePool(t, p)

	_, entries = rec.recorded()
	require.Equal(t, []string{"hello"}, entries)
}

func TestCmdOutputToggleWhileStarting(t *testing.T) {
	// a configuration reload can change LogOutput while commands are starting.
	p := &Pool{}
	p.Initialize()

	rec := &logRecorder{}

	stop := make(chan struct{})
	toggleDone := make(chan struct{})

	go func() {
		defer close(toggleDone)

		for v := false; ; v = !v {
			select {
			case <-stop:
				return
			default:
			}

			p.LogOutput.Store(v)
		}
	}()

	defer func() {
		close(stop)
		<-toggleDone
	}()

	for range 20 {
		cmd := &Cmd{
			Pool:         p,
			Cmdstr:       "true",
			OutputLogger: rec,
		}
		cmd.Start()
		time.Sleep(time.Millisecond)
	}
	closePool(t, p)
}

func TestCmdOutputStderrMerged(t *testing.T) {
	// the documented contract is that standard output and standard error are
	// merged into a single stream of entries. The command writes to the two
	// streams one after the other, so the bytes reach the shared pipe in
	// source order.
	//
	// the standard error tail is deliberately not terminated by '\n': such a
	// tail is only emitted by the close() of the logger, and runOSSpecific
	// closes the one that was returned to it, so the tail can reach the
	// recorder only while standard error is attached to that same logger. A
	// second logger attached to standard error that nobody closes would drop
	// the tail.
	p := &Pool{}
	p.Initialize()
	p.LogOutput.Store(true)

	rec := &logRecorder{}

	cmd := &Cmd{
		Pool:         p,
		Cmdstr:       "sh -c 'echo out; printf tail-err >&2'",
		OutputLogger: rec,
	}
	cmd.Start()
	closePool(t, p)

	_, entries := rec.recorded()
	require.Contains(t, entries, "out")
	require.Contains(t, entries, "tail-err")
}

func TestCmdOutputMissingLogger(t *testing.T) {
	// a command wired without an OutputLogger keeps inheriting the standard
	// streams of the server even while logging is enabled. A typed nil reaching
	// cmd.Stdout would make os/exec create a pipe and panic inside its copying
	// goroutine, taking down the server.
	out := filepath.Join(t.TempDir(), "out")
	f, err := os.Create(out)
	require.NoError(t, err)
	defer f.Close()

	prevStdout := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = prevStdout }()

	p := &Pool{}
	p.Initialize()
	p.LogOutput.Store(true)

	cmd := &Cmd{
		Pool:   p,
		Cmdstr: "sh -c 'echo x'",
	}
	cmd.Start()
	closePool(t, p)

	byts, err := os.ReadFile(out)
	require.NoError(t, err)
	require.Equal(t, "x\n", string(byts))
}

func TestCmdOutputOrphanShutdownBound(t *testing.T) {
	// both cases are bounded the same way; they differ in whether the
	// truncation is reported. Cmd.Wait returns ErrWaitDelay only when the
	// command itself exited successfully, and *ExitError has no Unwrap, so a
	// hook that failed hides the fact that its output was cut.
	for _, ca := range []struct {
		name     string
		exitCode int
		reported bool
	}{
		{"exit0", 0, true},
		{"exit1", 1, false},
	} {
		t.Run(ca.name, func(t *testing.T) {
			p := &Pool{}
			p.Initialize()
			p.LogOutput.Store(true)
			p.outputWaitDelay = 200 * time.Millisecond

			rec := &logRecorder{}

			cmd := &Cmd{
				Pool:         p,
				Cmdstr:       orphanCommand(t, "printf alive", ca.exitCode),
				OutputLogger: rec,
			}

			// the descendant inherits the output pipe and keeps it open, so the
			// copy can only end by expiry. The stopwatch starts before the
			// command exists, which makes the margin structurally positive.
			start := time.Now()
			cmd.Start()
			p.Close()
			elapsed := time.Since(start)

			require.GreaterOrEqual(t, elapsed, p.outputWaitDelay)
			require.Less(t, elapsed, p.outputWaitDelay+500*time.Millisecond)

			levels, entries := rec.recorded()
			require.Contains(t, entries, "alive")

			i := slices.Index(entries, "output copy did not complete within "+
				p.outputWaitDelay.String()+"; remaining output may be truncated")

			if !ca.reported {
				require.Equal(t, -1, i)
				return
			}

			// the level is asserted on the truncation entry itself, not on the
			// set of levels: every other entry of this test is logged at info
			// level.
			require.GreaterOrEqual(t, i, 0)
			require.Equal(t, logger.Debug, levels[i])

			// the pending partial line is flushed before the truncation is
			// reported.
			require.Less(t, slices.Index(entries, "alive"), i)
		})
	}
}

func TestCmdOutputSlowLoggerNotBounded(t *testing.T) {
	// the wait delay bounds the reading of the pipes, not a log write that is
	// already in progress: os/exec joins the copying goroutine unconditionally.
	var enteredOnce, releaseOnce sync.Once
	entered := make(chan struct{})
	release := make(chan struct{})

	// the writer stays blocked until release is closed, so it has to be closed on
	// every exit path, including the ones taken by t.Fatal.
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)

	// only the known "alive" entry is gated: the truncation diagnostic, that is
	// logged after Wait() returns, must not be delayed, otherwise the test would
	// pass even without any write in progress.
	rec := &logRecorder{onEntry: func(level logger.Level, entry string) {
		if level == logger.Info && entry == "alive" {
			enteredOnce.Do(func() { close(entered) })
			<-release
		}
	}}

	p := &Pool{}
	p.Initialize()
	p.LogOutput.Store(true)
	p.outputWaitDelay = 200 * time.Millisecond

	cmd := &Cmd{
		Pool:         p,
		Cmdstr:       orphanCommand(t, "echo alive", 0),
		OutputLogger: rec,
	}
	cmd.Start()

	done := make(chan struct{})
	go func() {
		p.Close()
		close(done)
	}()

	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout")
	}

	select {
	case <-done:
		t.Fatal("Pool.Close() returned while a log write was in progress")
	case <-time.After(p.outputWaitDelay + 500*time.Millisecond):
	}

	unblock()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout")
	}
}

func TestCmdOutputNoGoroutineLeak(t *testing.T) {
	p := &Pool{}
	p.Initialize()
	p.LogOutput.Store(true)

	rec := &logRecorder{}

	before := runtime.NumGoroutine()

	for range 5 {
		cmd := &Cmd{
			Pool:         p,
			Cmdstr:       "sh -c 'echo hello'",
			OutputLogger: rec,
		}
		cmd.Start()
	}
	closePool(t, p)

	_, entries := rec.recorded()
	require.Equal(t, 5, len(entries))

	// require.Eventually cannot be used here: it evaluates the condition in a
	// goroutine of its own, which the count would include.
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	require.LessOrEqual(t, runtime.NumGoroutine(), before)
}
