package hls

import (
	"os"
	"path/filepath"
	"time"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/hls"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/gin-gonic/gin"
)

type muxerInstance struct {
	variant         conf.HLSVariant
	segmentCount    int
	segmentDuration conf.Duration
	partDuration    conf.Duration
	segmentMaxSize  conf.StringSize
	directory       string
	pathName        string
	stream          *stream.Stream
	bytesSent       *uint64
	parent          logger.Writer

	hmuxer *gohlslib.Muxer
	reader *stream.Reader
}

func (mi *muxerInstance) initialize() error {
	var muxerDirectory string
	if mi.directory != "" {
		muxerDirectory = filepath.Join(mi.directory, mi.pathName)
		os.MkdirAll(muxerDirectory, 0o755)
	}

	mi.hmuxer = &gohlslib.Muxer{
		Variant:            gohlslib.MuxerVariant(mi.variant),
		SegmentCount:       mi.segmentCount,
		SegmentMinDuration: time.Duration(mi.segmentDuration),
		PartMinDuration:    time.Duration(mi.partDuration),
		SegmentMaxSize:     uint64(mi.segmentMaxSize),
		Directory:          muxerDirectory,
		OnEncodeError: func(err error) {
			mi.Log(logger.Warn, err.Error())
		},
	}

	mi.reader = &stream.Reader{
		SkipBytesSent: true,
		Parent:        mi,
	}

	err := hls.FromStream(mi.stream.Desc, mi.reader, mi.hmuxer)
	if err != nil {
		return err
	}

	err = mi.hmuxer.Start()
	if err != nil {
		return err
	}

	mi.Log(logger.Info, "is converting into HLS, %s",
		defs.FormatsInfo(mi.reader.Formats()))

	mi.stream.AddReader(mi.reader)

	return nil
}

// Log implements logger.Writer.
func (mi *muxerInstance) Log(level logger.Level, format string, args ...interface{}) {
	mi.parent.Log(level, format, args...)
}

func (mi *muxerInstance) close() {
	mi.stream.RemoveReader(mi.reader)
	mi.hmuxer.Close()
	if mi.hmuxer.Directory != "" {
		os.Remove(mi.hmuxer.Directory)
	}
}

func (mi *muxerInstance) errorChan() chan error {
	return mi.reader.Error()
}

func (mi *muxerInstance) handleRequest(ctx *gin.Context) {
	w := &responseWriterWithCounter{
		ResponseWriter: ctx.Writer,
		bytesSent:      mi.bytesSent,
	}

	mi.hmuxer.Handle(w, ctx.Request)
}
