package externalcmd

import (
	"bytes"
	"errors"
	"os/exec"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/bluenviron/mediamtx/internal/logger"
)

// defaultOutputWaitDelay bounds how long Cmd.Wait keeps reading the output pipes after
// the command itself has exited. Descendant processes can inherit the pipes and
// hold them open forever, so the wait has to be bounded; on expiry os/exec closes
// the pipes and any output still buffered in them is lost. It does not bound a
// log write that is already in progress: os/exec joins the copying goroutine
// unconditionally after closing the pipes.
// 1s is a compromise: far below the shutdown grace periods of docker and
// supervisord (10s). A hook that leaves background descendants behind
// (sh -c '... &') can keep the pipes open for as long as they live, which is
// exactly what this limit exists for. It is deliberately not derived from the
// capacity of a pipe: that capacity is not part of the interface (see pipe(7)).
const defaultOutputWaitDelay = time.Second

// maxOutputLineLength is the maximum size of a single log entry produced from
// command output. A longer line is split into several entries, and the buffer
// never grows beyond it, so a command that never emits '\n' cannot exhaust memory.
const maxOutputLineLength = 4096

// outputLogger turns the output of a command into log entries, one per line;
// lines longer than maxOutputLineLength are split.
type outputLogger struct {
	dest      logger.Writer
	waitDelay time.Duration

	mutex  sync.Mutex
	buf    []byte
	closed bool
}

// attachOutputLogger routes the output of cmd to c.OutputLogger when hook
// output logging is enabled, and returns the logger so that the caller can
// close it. It returns nil when logging is disabled: in that case cmd.Stdout,
// cmd.Stderr and cmd.WaitDelay are left exactly as the caller set them, so the
// command keeps inheriting the standard streams of the server.
func (c *Cmd) attachOutputLogger(cmd *exec.Cmd) *outputLogger {
	if !c.Pool.LogOutput.Load() || c.OutputLogger == nil {
		return nil
	}

	delay := c.Pool.outputWaitDelay
	if delay == 0 {
		delay = defaultOutputWaitDelay
	}

	ol := &outputLogger{dest: c.OutputLogger, waitDelay: delay}
	cmd.Stdout = ol
	cmd.Stderr = ol       // same pointer: os/exec reuses a single pipe when Stdout == Stderr
	cmd.WaitDelay = delay // bounds reading, not the logger write in flight

	return ol
}

// Write implements io.Writer.
func (w *outputLogger) Write(p []byte) (int, error) {
	n := len(p)

	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.closed {
		return n, nil
	}

	for {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			w.appendLocked(p)
			break
		}

		w.appendLocked(p[:i])
		w.emitLocked(true)
		p = p[i+1:]
	}

	return n, nil
}

// close flushes the pending partial line and permanently stops logging.
// It can be called more than once. After close returns, Write never touches
// dest again.
func (w *outputLogger) close() {
	if w == nil {
		return
	}

	w.mutex.Lock()
	defer w.mutex.Unlock()

	w.emitLocked(false)
	w.closed = true
}

// reportTruncation logs the fact that the output copy did not finish within
// the wait delay after the command exited, so part of the output was dropped.
// This happens when descendant processes keep the output pipes open, and also
// when the log destinations are slower than the output the command produced.
//
// It only fires when the command exited successfully: Cmd.Wait reports
// ErrWaitDelay only if the command itself returned no error,
// and *ExitError has no Unwrap, so a command that failed masks it.
func (w *outputLogger) reportTruncation(err error) {
	if w == nil || !errors.Is(err, exec.ErrWaitDelay) {
		return
	}

	w.dest.Log(logger.Debug, "output copy did not complete within %v; "+
		"remaining output may be truncated", w.waitDelay)
}

// appendLocked keeps the buffer bounded, inserting a hard record boundary
// when a line exceeds maxOutputLineLength. The boundary never falls inside a
// multi-byte UTF-8 sequence: an incomplete sequence at the end of the record
// is moved to the next one.
func (w *outputLogger) appendLocked(p []byte) {
	for len(w.buf)+len(p) > maxOutputLineLength {
		k := maxOutputLineLength - len(w.buf)
		w.buf = append(w.buf, p[:k]...)
		p = p[k:]

		n := len(w.buf) - incompleteTail(w.buf)
		rest := append([]byte(nil), w.buf[n:]...)
		w.buf = w.buf[:n]
		w.emitLocked(false)
		w.buf = append(w.buf, rest...)
	}

	w.buf = append(w.buf, p...)
}

// incompleteTail returns the length of the multi-byte UTF-8 sequence that
// starts within the last bytes of b and is not complete, or 0.
func incompleteTail(b []byte) int {
	for i := 1; i < utf8.UTFMax && i <= len(b); i++ {
		if !utf8.RuneStart(b[len(b)-i]) {
			continue
		}
		if !utf8.FullRune(b[len(b)-i:]) {
			return i
		}
		return 0
	}
	return 0
}

// emitLocked writes the buffer as one log entry. stripCR must be true only
// when the record was terminated by '\n', so that CRLF collapses to LF.
func (w *outputLogger) emitLocked(stripCR bool) {
	line := w.buf
	if stripCR {
		line = bytes.TrimSuffix(line, []byte("\r"))
	}

	if len(line) != 0 {
		w.dest.Log(logger.Info, "%s", string(line))
	}

	w.buf = w.buf[:0]
}
