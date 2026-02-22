package rtsp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/errordumper"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/mpegts"
	"github.com/bluenviron/mediamtx/internal/stream"
)

const (
	mpegtsInitTimeout = 10 * time.Second
)

type serverPathManager interface {
	AddPublisher(req defs.PathAddPublisherReq) (defs.Path, *stream.SubStream, error)
}

// MPEGTSDemuxer demuxes an MPEG-TS stream received via RTP into component streams.
type MPEGTSDemuxer struct {
	rsession     *gortsplib.ServerSession
	pathManager  serverPathManager
	pathConf     *conf.Path
	author       defs.Publisher
	logger       logger.Writer
	decodeErrors *errordumper.Dumper
	pathName     string
	query        string

	ctx        context.Context
	ctxCancel  context.CancelFunc
	pipeWriter *io.PipeWriter
	path       defs.Path
	subStream  *stream.SubStream
	initDone   chan error
	loopErr    chan error
}

// NewMPEGTSDemuxer creates a new MPEGTSDemuxer.
func NewMPEGTSDemuxer(
	rsession *gortsplib.ServerSession,
	pathManager serverPathManager,
	pathConf *conf.Path,
	author defs.Publisher,
	l logger.Writer,
	decodeErrors *errordumper.Dumper,
	pathName string,
	query string,
) *MPEGTSDemuxer {
	ctx, ctxCancel := context.WithCancel(context.Background())

	return &MPEGTSDemuxer{
		rsession:     rsession,
		pathManager:  pathManager,
		pathConf:     pathConf,
		author:       author,
		logger:       l,
		decodeErrors: decodeErrors,
		pathName:     pathName,
		query:        query,
		ctx:          ctx,
		ctxCancel:    ctxCancel,
		initDone:     make(chan error, 1),
		loopErr:      make(chan error, 1),
	}
}

// Start begins the demuxing process.
func (d *MPEGTSDemuxer) Start() {
	medias := d.rsession.AnnouncedDescription().Medias
	if len(medias) != 1 || len(medias[0].Formats) != 1 {
		d.initDone <- errors.New("expected exactly one MPEG-TS track")
		return
	}
	medi := medias[0]
	forma, ok := medi.Formats[0].(*format.MPEGTS)
	if !ok {
		d.initDone <- errors.New("expected MPEG-TS format")
		return
	}

	decoder, err := forma.CreateDecoder()
	if err != nil {
		d.initDone <- fmt.Errorf("failed to create MPEG-TS decoder: %w", err)
		return
	}

	pr, pw := io.Pipe()
	d.pipeWriter = pw

	d.rsession.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
		tsData, err := decoder.Decode(pkt)
		if err != nil {
			d.decodeErrors.Add(err)
			return
		}
		pw.Write(tsData) //nolint:errcheck
	})

	go d.run(pr)
}

// WaitForInit blocks until initialization completes or times out.
func (d *MPEGTSDemuxer) WaitForInit() (defs.Path, *stream.SubStream, error) {
	select {
	case err := <-d.initDone:
		if err != nil {
			d.ctxCancel()
			return nil, nil, fmt.Errorf("MPEG-TS initialization failed: %w", err)
		}
		return d.path, d.subStream, nil

	case <-time.After(mpegtsInitTimeout):
		d.ctxCancel()
		d.pipeWriter.CloseWithError(errors.New("initialization timeout"))
		return nil, nil, errors.New("MPEG-TS initialization timeout: PMT not received")

	case <-d.ctx.Done():
		return nil, nil, errors.New("demuxer stopped")
	}
}

func (d *MPEGTSDemuxer) run(pr *io.PipeReader) {
	err := d.doRun(pr)
	if err != nil {
		d.initDone <- err
	}
}

func (d *MPEGTSDemuxer) doRun(pr *io.PipeReader) error {
	r := &mpegts.EnhancedReader{R: pr}
	err := r.Initialize()
	if err != nil {
		return fmt.Errorf("failed to initialize MPEG-TS reader: %w", err)
	}

	r.OnDecodeError(func(err error) {
		d.decodeErrors.Add(err)
	})

	medias, err := mpegts.ToStream(r, &d.subStream, d.logger)
	if err != nil {
		return fmt.Errorf("failed to map MPEG-TS to stream: %w", err)
	}

	d.logger.Log(logger.Info, "MPEG-TS demux discovered %d tracks", len(medias))

	d.path, d.subStream, err = d.pathManager.AddPublisher(defs.PathAddPublisherReq{
		Author:        d.author,
		Desc:          &description.Session{Medias: medias},
		UseRTPPackets: false,
		ReplaceNTP:    true,
		ConfToCompare: d.pathConf,
		AccessRequest: defs.PathAccessRequest{
			Name:     d.pathName,
			Query:    d.query,
			Publish:  true,
			SkipAuth: true,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add publisher: %w", err)
	}

	d.initDone <- nil

	for {
		err = r.Read()
		if err != nil {
			d.loopErr <- err
			return nil
		}
	}
}

// Stop stops the demuxer.
func (d *MPEGTSDemuxer) Stop() {
	d.ctxCancel()
	d.pipeWriter.CloseWithError(errors.New("demuxer stopped"))
}

// WaitForLoopError blocks until the read loop exits with an error.
func (d *MPEGTSDemuxer) WaitForLoopError() error {
	select {
	case err := <-d.loopErr:
		return err
	case <-d.ctx.Done():
		return nil
	}
}

// IsMPEGTSFormat checks if a format is MPEG-TS.
func IsMPEGTSFormat(forma format.Format) bool {
	return forma.Codec() == "MPEG-TS"
}
