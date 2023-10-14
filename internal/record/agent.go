package record

import (
	"context"
	"strings"
	"time"

	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"

	"github.com/bluenviron/mediamtx/internal/asyncwriter"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
)

// OnSegmentFunc is the prototype of the function passed as runOnSegmentStart / runOnSegmentComplete
type OnSegmentFunc = func(string)

type sample struct {
	*fmp4.PartSample
	dts time.Duration
}

// Agent saves streams on disk.
type Agent struct {
	path              string
	partDuration      time.Duration
	segmentDuration   time.Duration
	stream            *stream.Stream
	onSegmentCreate   OnSegmentFunc
	onSegmentComplete OnSegmentFunc
	parent            logger.Writer

	ctx       context.Context
	ctxCancel func()
	writer    *asyncwriter.Writer
	format    recFormat

	done chan struct{}
}

// NewAgent allocates an Agent.
func NewAgent(
	writeQueueSize int,
	path string,
	format conf.RecordFormat,
	partDuration time.Duration,
	segmentDuration time.Duration,
	pathName string,
	stream *stream.Stream,
	onSegmentCreate OnSegmentFunc,
	onSegmentComplete OnSegmentFunc,
	parent logger.Writer,
) *Agent {
	path = strings.ReplaceAll(path, "%path", pathName)

	switch format {
	case conf.RecordFormatMPEGTS:
		path += ".ts"

	default:
		path += ".mp4"
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	a := &Agent{
		path:              path,
		partDuration:      partDuration,
		segmentDuration:   segmentDuration,
		stream:            stream,
		onSegmentCreate:   onSegmentCreate,
		onSegmentComplete: onSegmentComplete,
		parent:            parent,
		ctx:               ctx,
		ctxCancel:         ctxCancel,
		done:              make(chan struct{}),
	}

	a.writer = asyncwriter.New(writeQueueSize, a)

	switch format {
	case conf.RecordFormatMPEGTS:
		a.format = newRecFormatMPEGTS(a)

	default:
		a.format = newRecFormatFMP4(a)
	}

	go a.run()

	return a
}

// Close closes the Agent.
func (a *Agent) Close() {
	a.Log(logger.Info, "recording stopped")

	a.ctxCancel()
	<-a.done
}

// Log is the main logging function.
func (a *Agent) Log(level logger.Level, format string, args ...interface{}) {
	a.parent.Log(level, "[record] "+format, args...)
}

func (a *Agent) run() {
	defer close(a.done)

	a.writer.Start()

	select {
	case err := <-a.writer.Error():
		a.Log(logger.Error, err.Error())
		a.stream.RemoveReader(a.writer)

	case <-a.ctx.Done():
		a.stream.RemoveReader(a.writer)
		a.writer.Stop()
	}

	a.format.close()
}
