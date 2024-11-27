package stream

import (
	"fmt"

	"github.com/bluenviron/gortsplib/v4/pkg/ringbuffer"
	"github.com/bluenviron/mediamtx/internal/logger"
)

type streamReader struct {
	queueSize int
	parent    logger.Writer

	writeErrLogger logger.Writer
	buffer         *ringbuffer.RingBuffer
	started        bool

	// out
	err chan error
}

func (w *streamReader) initialize() {
	w.writeErrLogger = logger.NewLimitedLogger(w.parent)
	buffer, _ := ringbuffer.New(uint64(w.queueSize))
	w.buffer = buffer
	w.err = make(chan error)
}

func (w *streamReader) start() {
	w.started = true
	go w.run()
}

func (w *streamReader) stop() {
	w.buffer.Close()
	if w.started {
		<-w.err
	}
}

func (w *streamReader) error() chan error {
	return w.err
}

func (w *streamReader) run() {
	w.err <- w.runInner()
	close(w.err)
}

func (w *streamReader) runInner() error {
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

func (w *streamReader) push(cb func() error) {
	ok := w.buffer.Push(cb)
	if !ok {
		w.writeErrLogger.Log(logger.Warn, "write queue is full")
	}
}
