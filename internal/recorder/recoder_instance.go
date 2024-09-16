package recorder

import (
	"strings"
	"time"

	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"

	"github.com/bluenviron/mediamtx/internal/asyncwriter"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/recordstore"
)

type sample struct {
	*fmp4.PartSample
	dts time.Duration
	ntp time.Time
}

type agentInstance struct {
	agent *Recorder

	pathFormat string
	writer     *asyncwriter.Writer
	format     format

	terminate chan struct{}
	done      chan struct{}
}

// Log implements logger.Writer.
func (ai *agentInstance) Log(level logger.Level, format string, args ...interface{}) {
	ai.agent.Log(level, format, args...)
}

func (ai *agentInstance) initialize() {
	ai.pathFormat = ai.agent.PathFormat

	ai.pathFormat = recordstore.PathAddExtension(
		strings.ReplaceAll(ai.pathFormat, "%path", ai.agent.PathName),
		ai.agent.Format,
	)

	ai.terminate = make(chan struct{})
	ai.done = make(chan struct{})

	ai.writer = asyncwriter.New(ai.agent.WriteQueueSize, ai.agent)

	switch ai.agent.Format {
	case conf.RecordFormatMPEGTS:
		ai.format = &formatMPEGTS{
			ai: ai,
		}
		ai.format.initialize()

	default:
		ai.format = &formatFMP4{
			ai: ai,
		}
		ai.format.initialize()
	}

	go ai.run()
}

func (ai *agentInstance) close() {
	close(ai.terminate)
	<-ai.done
}

func (ai *agentInstance) run() {
	defer close(ai.done)

	ai.writer.Start()

	select {
	case err := <-ai.writer.Error():
		ai.Log(logger.Error, err.Error())
		ai.agent.Stream.RemoveReader(ai.writer)

	case <-ai.terminate:
		ai.agent.Stream.RemoveReader(ai.writer)
		ai.writer.Stop()
	}

	ai.format.close()
}
