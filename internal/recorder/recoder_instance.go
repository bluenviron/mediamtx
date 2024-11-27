package recorder

import (
	"strings"
	"time"

	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/recordstore"
)

type sample struct {
	*fmp4.PartSample
	dts int64
	ntp time.Time
}

type recorderInstance struct {
	agent *Recorder

	pathFormat string
	format     format

	terminate chan struct{}
	done      chan struct{}
}

// Log implements logger.Writer.
func (ai *recorderInstance) Log(level logger.Level, format string, args ...interface{}) {
	ai.agent.Log(level, format, args...)
}

func (ai *recorderInstance) initialize() {
	ai.pathFormat = ai.agent.PathFormat

	ai.pathFormat = recordstore.PathAddExtension(
		strings.ReplaceAll(ai.pathFormat, "%path", ai.agent.PathName),
		ai.agent.Format,
	)

	ai.terminate = make(chan struct{})
	ai.done = make(chan struct{})

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

	ai.agent.Stream.StartReader(ai)

	go ai.run()
}

func (ai *recorderInstance) close() {
	close(ai.terminate)
	<-ai.done
}

func (ai *recorderInstance) run() {
	defer close(ai.done)

	select {
	case err := <-ai.agent.Stream.ReaderError(ai):
		ai.Log(logger.Error, err.Error())

	case <-ai.terminate:
	}

	ai.agent.Stream.RemoveReader(ai)

	ai.format.close()
}
