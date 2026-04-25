package hls

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/hls"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/gin-gonic/gin"
)

const (
	sessionCookieName     = "hlsSession"
	sessionQueryParamName = "session"
	sessionCloseAfter     = 30 * time.Second
	sessionCleanupPeriod  = sessionCloseAfter / 3
)

type instanceParent interface {
	logger.Writer
	closeInstance(*muxerInstance, error)
}

type muxerInstance struct {
	variant         conf.HLSVariant
	segmentCount    int
	segmentDuration conf.Duration
	partDuration    conf.Duration
	segmentMaxSize  conf.StringSize
	directory       string
	pathName        string
	bytesSent       *atomic.Uint64
	wg              *sync.WaitGroup
	stream          *stream.Stream
	server          logger.Writer
	parent          instanceParent

	ctx       context.Context
	ctxCancel func()
	hmuxer    *gohlslib.Muxer
	reader    *stream.Reader
}

func (mi *muxerInstance) initialize() error {
	mi.Log(logger.Debug, "instance created")

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
		SkipOutboundBytes: true,
		Parent:            mi,
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

	mi.ctx, mi.ctxCancel = context.WithCancel(context.Background())

	mi.wg.Add(1)
	go mi.run()

	return nil
}

// Log implements logger.Writer.
func (mi *muxerInstance) Log(level logger.Level, format string, args ...any) {
	mi.parent.Log(level, format, args...)
}

func (mi *muxerInstance) close() {
	mi.ctxCancel()
}

func (mi *muxerInstance) run() {
	defer mi.wg.Done()

	err := mi.runInner()

	mi.ctxCancel()

	mi.stream.RemoveReader(mi.reader)

	mi.hmuxer.Close()

	if mi.hmuxer.Directory != "" {
		os.Remove(mi.hmuxer.Directory)
	}

	mi.Log(logger.Debug, "instance destroyed: %v", err)

	mi.parent.closeInstance(mi, err)
}

func (mi *muxerInstance) runInner() error {
	for {
		select {
		case <-mi.ctx.Done():
			return fmt.Errorf("terminated")

		case err := <-mi.reader.Error():
			return err
		}
	}
}

func (mi *muxerInstance) handleRequest(ctx *gin.Context) {
	w := ctx.Writer

	w = &responseWriterNoCache{ResponseWriter: w}

	w = &responseWriterCounter{
		ResponseWriter: w,
		bytesSent:      mi.bytesSent,
	}

	mi.hmuxer.Handle(w, ctx.Request)
}
