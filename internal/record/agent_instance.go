package record

import (
	"strings"
	"time"

	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"

	"github.com/bluenviron/mediamtx/internal/asyncwriter"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// OnSegmentFunc is the prototype of the function passed as runOnSegmentStart / runOnSegmentComplete
type OnSegmentFunc = func(string)

type sample struct {
	*fmp4.PartSample
	dts time.Duration
}

type agentInstance struct {
	wrapper *Agent

	resolvedPath string
	writer       *asyncwriter.Writer
	format       recFormat

	terminate chan struct{}
	done      chan struct{}
}

func (a *agentInstance) initialize() {
	a.resolvedPath = strings.ReplaceAll(a.wrapper.RecordPath, "%path", a.wrapper.PathName)

	switch a.wrapper.Format {
	case conf.RecordFormatMPEGTS:
		a.resolvedPath += ".ts"

	default:
		a.resolvedPath += ".mp4"
	}

	a.terminate = make(chan struct{})
	a.done = make(chan struct{})

	a.writer = asyncwriter.New(a.wrapper.WriteQueueSize, a.wrapper)

	switch a.wrapper.Format {
	case conf.RecordFormatMPEGTS:
		a.format = &recFormatMPEGTS{
			a: a,
		}
		a.format.initialize()

	default:
		a.format = &recFormatFMP4{
			a: a,
		}
		a.format.initialize()
	}

	go a.run()
}

func (a *agentInstance) close() {
	close(a.terminate)
	<-a.done
}

func (a *agentInstance) run() {
	defer close(a.done)

	a.writer.Start()

	select {
	case err := <-a.writer.Error():
		a.wrapper.Log(logger.Error, err.Error())
		a.wrapper.Stream.RemoveReader(a.writer)

	case <-a.terminate:
		a.wrapper.Stream.RemoveReader(a.writer)
		a.writer.Stop()
	}

	a.format.close()
}
