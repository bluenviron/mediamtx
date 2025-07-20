package recorder

import (
	"strings"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/recordstore"
	"github.com/bluenviron/mediamtx/internal/stream"
)

type sample struct {
	*fmp4.Sample
	dts int64
	ntp time.Time
}

type recorderInstance struct {
	pathFormat        string
	format            conf.RecordFormat
	partDuration      time.Duration
	maxPartSize       conf.StringSize
	segmentDuration   time.Duration
	pathName          string
	stream            *stream.Stream
	onSegmentCreate   OnSegmentCreateFunc
	onSegmentComplete OnSegmentCompleteFunc
	parent            logger.Writer

	pathFormat2 string
	format2     format
	skip        bool

	terminate chan struct{}
	done      chan struct{}
}

// Log implements logger.Writer.
func (ri *recorderInstance) Log(level logger.Level, format string, args ...interface{}) {
	ri.parent.Log(level, format, args...)
}

func (ri *recorderInstance) initialize() {
	ri.pathFormat2 = ri.pathFormat

	ri.pathFormat2 = recordstore.PathAddExtension(
		strings.ReplaceAll(ri.pathFormat2, "%path", ri.pathName),
		ri.format,
	)

	ri.terminate = make(chan struct{})
	ri.done = make(chan struct{})

	switch ri.format {
	case conf.RecordFormatMPEGTS:
		ri.format2 = &formatMPEGTS{
			ri: ri,
		}
		ok := ri.format2.initialize()
		ri.skip = !ok

	default:
		ri.format2 = &formatFMP4{
			ri: ri,
		}
		ok := ri.format2.initialize()
		ri.skip = !ok
	}

	if !ri.skip {
		ri.stream.StartReader(ri)
	}

	go ri.run()
}

func (ri *recorderInstance) close() {
	close(ri.terminate)
	<-ri.done
}

func (ri *recorderInstance) run() {
	defer close(ri.done)

	if !ri.skip {
		select {
		case err := <-ri.stream.ReaderError(ri):
			ri.Log(logger.Error, err.Error())

		case <-ri.terminate:
		}

		ri.stream.RemoveReader(ri)
	} else {
		<-ri.terminate
	}

	ri.format2.close()
}
