package hls

import (
	"os"
	"path/filepath"
	"time"

	"github.com/bluenviron/gohlslib"
	"github.com/bluenviron/mediamtx/internal/asyncwriter"
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
	segmentDuration conf.StringDuration
	partDuration    conf.StringDuration
	segmentMaxSize  conf.StringSize
	directory       string
	writeQueueSize  int
	pathName        string
	stream          *stream.Stream
	bytesSent       *uint64
	parent          logger.Writer

	writer *asyncwriter.Writer
	hmuxer *gohlslib.Muxer
}

func (mi *muxerInstance) initialize() error {
	mi.writer = asyncwriter.New(mi.writeQueueSize, mi)

	var muxerDirectory string
	if mi.directory != "" {
		muxerDirectory = filepath.Join(mi.directory, mi.pathName)
		os.MkdirAll(muxerDirectory, 0o755)
	}

	mi.hmuxer = &gohlslib.Muxer{
		Variant:         gohlslib.MuxerVariant(mi.variant),
		SegmentCount:    mi.segmentCount,
		SegmentDuration: time.Duration(mi.segmentDuration),
		PartDuration:    time.Duration(mi.partDuration),
		SegmentMaxSize:  uint64(mi.segmentMaxSize),
		Directory:       muxerDirectory,
	}

	err := hls.FromStream(mi.stream, mi.writer, mi.hmuxer, mi)
	if err != nil {
		mi.stream.RemoveReader(mi.writer)
		return err
	}

	err = mi.hmuxer.Start()
	if err != nil {
		mi.stream.RemoveReader(mi.writer)
		return err
	}

	mi.Log(logger.Info, "is converting into HLS, %s",
		defs.FormatsInfo(mi.stream.FormatsForReader(mi.writer)))

	mi.writer.Start()

	return nil
}

// Log implements logger.Writer.
func (mi *muxerInstance) Log(level logger.Level, format string, args ...interface{}) {
	mi.parent.Log(level, format, args...)
}

func (mi *muxerInstance) close() {
	mi.writer.Stop()
	mi.hmuxer.Close()
	mi.stream.RemoveReader(mi.writer)
	if mi.hmuxer.Directory != "" {
		os.Remove(mi.hmuxer.Directory)
	}
}

func (mi *muxerInstance) errorChan() chan error {
	return mi.writer.Error()
}

func (mi *muxerInstance) handleRequest(ctx *gin.Context) {
	w := &responseWriterWithCounter{
		ResponseWriter: ctx.Writer,
		bytesSent:      mi.bytesSent,
	}

	mi.hmuxer.Handle(w, ctx.Request)
}
