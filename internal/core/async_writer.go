package core

import (
	"fmt"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/ringbuffer"

	"github.com/bluenviron/mediamtx/internal/logger"
)

const (
	minIntervalBetweenWarnings = 1 * time.Second
)

type asyncWriter struct {
	parent logger.Writer

	buffer               *ringbuffer.RingBuffer
	prevWarnPrinted      time.Time
	prevWarnPrintedMutex sync.Mutex

	// out
	err chan error
}

func newAsyncWriter(
	queueSize int,
	parent logger.Writer,
) *asyncWriter {
	buffer, _ := ringbuffer.New(uint64(queueSize))

	return &asyncWriter{
		parent: parent,
		buffer: buffer,
		err:    make(chan error),
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
		now := time.Now()
		w.prevWarnPrintedMutex.Lock()
		if now.Sub(w.prevWarnPrinted) >= minIntervalBetweenWarnings {
			w.prevWarnPrinted = now
			w.parent.Log(logger.Warn, "write queue is full")
		}
		w.prevWarnPrintedMutex.Unlock()
	}
}
