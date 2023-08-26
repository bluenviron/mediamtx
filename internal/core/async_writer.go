package core

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/ringbuffer"

	"github.com/bluenviron/mediamtx/internal/logger"
)

const (
	minIntervalBetweenWarnings = 1 * time.Second
)

type asyncWriter struct {
	writeErrLogger logger.Writer
	buffer         *ringbuffer.RingBuffer

	// out
	err chan error
}

func newAsyncWriter(
	queueSize int,
	parent logger.Writer,
) *asyncWriter {
	buffer, _ := ringbuffer.New(uint64(queueSize))

	return &asyncWriter{
		writeErrLogger: newLimitedLogger(parent),
		buffer:         buffer,
		err:            make(chan error),
	}
}

func (w *asyncWriter) start() {
	go w.run()
}

func (w *asyncWriter) stop() {
	w.buffer.Close()
	<-w.err
}

func (w *asyncWriter) error() chan error {
	return w.err
}

func (w *asyncWriter) run() {
	w.err <- w.runInner()
}

func (w *asyncWriter) runInner() error {
	for {
		cb, ok := w.buffer.Pull()
		if !ok {
			return fmt.Errorf("terminated")
		}

		err := cb.(func() error)()
		if err != nil {
			return err
		}
	}
}

func (w *asyncWriter) push(cb func() error) {
	ok := w.buffer.Push(cb)
	if !ok {
		w.writeErrLogger.Log(logger.Warn, "write queue is full")
	}
}
