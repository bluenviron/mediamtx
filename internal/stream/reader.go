package stream

import (
	"fmt"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/ringbuffer"
	"github.com/bluenviron/mediamtx/internal/counterdumper"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// OnDataFunc is the callback passed to OnData().
type OnDataFunc func(*unit.Unit) error

// Reader is a stream reader.
type Reader struct {
	SkipBytesSent bool
	Parent        logger.Writer

	onDatas         map[*description.Media]map[format.Format]OnDataFunc
	queueSize       int
	buffer          *ringbuffer.RingBuffer
	discardedFrames *counterdumper.Dumper

	// out
	err chan error
}

// OnData registers a callback that is called when data from given format is available.
func (r *Reader) OnData(medi *description.Media, forma format.Format, cb OnDataFunc) {
	if r.onDatas == nil {
		r.onDatas = make(map[*description.Media]map[format.Format]OnDataFunc)
	}
	if r.onDatas[medi] == nil {
		r.onDatas[medi] = make(map[format.Format]OnDataFunc)
	}
	r.onDatas[medi][forma] = cb
}

// Formats returns all formats for which the reader has registered a OnData callback.
func (r *Reader) Formats() []format.Format {
	n := 0
	for _, formats := range r.onDatas {
		for range formats {
			n++
		}
	}

	if n == 0 {
		return nil
	}

	out := make([]format.Format, n)
	n = 0

	for _, formats := range r.onDatas {
		for forma := range formats {
			out[n] = forma
			n++
		}
	}

	return out
}

// error returns whenever there's an error.
// It can be called only after stream.AddReader().
func (r *Reader) Error() chan error {
	return r.err
}

func (r *Reader) start() {
	buffer, _ := ringbuffer.New(uint64(r.queueSize))
	r.buffer = buffer
	r.err = make(chan error)

	r.discardedFrames = &counterdumper.Dumper{
		OnReport: func(val uint64) {
			r.Parent.Log(logger.Warn, "reader is too slow, discarding %d %s",
				val,
				func() string {
					if val == 1 {
						return "frame"
					}
					return "frames"
				}())
		},
	}
	r.discardedFrames.Start()

	go r.run()
}

func (r *Reader) stop() {
	r.buffer.Close()
	r.discardedFrames.Stop()
	<-r.err
}

func (r *Reader) run() {
	r.err <- r.runInner()
	close(r.err)
}

func (r *Reader) runInner() error {
	for {
		cb, ok := r.buffer.Pull()
		if !ok {
			return fmt.Errorf("terminated")
		}

		err := cb.(func() error)()
		if err != nil {
			return err
		}
	}
}

func (r *Reader) push(cb func() error) {
	ok := r.buffer.Push(cb)
	if !ok {
		r.discardedFrames.Increase()
	}
}
