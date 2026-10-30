package externalcmd

import (
	"bytes"
	"os/exec"
	"time"

	"github.com/bluenviron/mediamtx/internal/logger"
)

// bound the wait for output, since background processes can keep the pipe open.
const outputWaitDelay = time.Second

// bound the buffer, since a command can write without ever writing '\n'.
const maxOutputLineLength = 4096

type outputLogger struct {
	parent logger.Writer
	buf    []byte
}

func (c *Cmd) attachOutputLogger(cmd *exec.Cmd) *outputLogger {
	c.Pool.mutex.RLock()
	logOutput := c.Pool.LogOutput
	c.Pool.mutex.RUnlock()

	if !logOutput {
		return nil
	}

	ol := &outputLogger{parent: c.OutputLogger}

	// a single writer makes os/exec use a single pipe and a single goroutine.
	cmd.Stdout = ol
	cmd.Stderr = ol
	cmd.WaitDelay = outputWaitDelay

	return ol
}

// Write implements io.Writer.
func (w *outputLogger) Write(p []byte) (int, error) {
	n := len(p)

	for {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			break
		}

		w.append(p[:i])
		w.buf = bytes.TrimSuffix(w.buf, []byte("\r"))
		w.flush()
		p = p[i+1:]
	}

	w.append(p)

	return n, nil
}

func (w *outputLogger) append(p []byte) {
	for len(w.buf)+len(p) > maxOutputLineLength {
		k := maxOutputLineLength - len(w.buf)
		w.buf = append(w.buf, p[:k]...)
		w.flush()
		p = p[k:]
	}

	w.buf = append(w.buf, p...)
}

func (w *outputLogger) flush() {
	if len(w.buf) != 0 {
		w.parent.Log(logger.Info, "%s", string(w.buf))
	}

	w.buf = w.buf[:0]
}
