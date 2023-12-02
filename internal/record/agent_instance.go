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
	agent *Agent

	segmentPathFormat string
	writer            *asyncwriter.Writer
	format            format

	terminate chan struct{}
	done      chan struct{}
}

func (a *agentInstance) initialize() {
	a.segmentPathFormat = a.agent.SegmentPathFormat

	a.segmentPathFormat = strings.ReplaceAll(a.segmentPathFormat, "%path", a.agent.PathName)

	switch a.agent.Format {
	case conf.RecordFormatMPEGTS:
		a.segmentPathFormat += ".ts"

	default:
		a.segmentPathFormat += ".mp4"
	}

	a.terminate = make(chan struct{})
	a.done = make(chan struct{})

	a.writer = asyncwriter.New(a.agent.WriteQueueSize, a.agent)

	switch a.agent.Format {
	case conf.RecordFormatMPEGTS:
		a.format = &formatMPEGTS{
			a: a,
		}
		a.format.initialize()

	default:
		a.format = &formatFMP4{
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
		a.agent.Log(logger.Error, err.Error())
		a.agent.Stream.RemoveReader(a.writer)

	case <-a.terminate:
		a.agent.Stream.RemoveReader(a.writer)
		a.writer.Stop()
	}

	a.format.close()
}
