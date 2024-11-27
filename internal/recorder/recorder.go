// Package recorder contains the recorder.
package recorder

import (
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
)

// OnSegmentCreateFunc is the prototype of the function passed as OnSegmentCreate
type OnSegmentCreateFunc = func(path string)

// OnSegmentCompleteFunc is the prototype of the function passed as OnSegmentComplete
type OnSegmentCompleteFunc = func(path string, duration time.Duration)

// Recorder writes recordings to disk.
type Recorder struct {
	PathFormat        string
	Format            conf.RecordFormat
	PartDuration      time.Duration
	SegmentDuration   time.Duration
	PathName          string
	Stream            *stream.Stream
	OnSegmentCreate   OnSegmentCreateFunc
	OnSegmentComplete OnSegmentCompleteFunc
	Parent            logger.Writer

	restartPause time.Duration

	currentInstance *recorderInstance

	terminate chan struct{}
	done      chan struct{}
}

// Initialize initializes Recorder.
func (w *Recorder) Initialize() {
	if w.OnSegmentCreate == nil {
		w.OnSegmentCreate = func(string) {
		}
	}
	if w.OnSegmentComplete == nil {
		w.OnSegmentComplete = func(string, time.Duration) {
		}
	}
	if w.restartPause == 0 {
		w.restartPause = 2 * time.Second
	}

	w.terminate = make(chan struct{})
	w.done = make(chan struct{})

	w.currentInstance = &recorderInstance{
		agent: w,
	}
	w.currentInstance.initialize()

	go w.run()
}

// Log implements logger.Writer.
func (w *Recorder) Log(level logger.Level, format string, args ...interface{}) {
	w.Parent.Log(level, "[recorder] "+format, args...)
}

// Close closes the agent.
func (w *Recorder) Close() {
	w.Log(logger.Info, "recording stopped")
	close(w.terminate)
	<-w.done
}

func (w *Recorder) run() {
	defer close(w.done)

	for {
		select {
		case <-w.currentInstance.done:
			w.currentInstance.close()
		case <-w.terminate:
			w.currentInstance.close()
			return
		}

		select {
		case <-time.After(w.restartPause):
		case <-w.terminate:
			return
		}

		w.currentInstance = &recorderInstance{
			agent: w,
		}
		w.currentInstance.initialize()
	}
}
