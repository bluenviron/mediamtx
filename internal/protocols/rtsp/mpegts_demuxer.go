package rtsp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/counterdumper"
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

type MPEGTSDemuxer struct {
	rsession     *gortsplib.ServerSession
	pathManager  serverPathManager
	pathConf     *conf.Path
	author       defs.Publisher
	logger       logger.Writer
	packetsLost  *counterdumper.Dumper
	decodeErrors *errordumper.Dumper
	pathName     string
	query        string

	ctx        context.Context
	ctxCancel  context.CancelFunc
	rtpReader  *rtpMPEGTSReader
	tsReader   *mpegts.EnhancedReader
	path       defs.Path
	subStream  *stream.SubStream
	initDone   chan error
	loopErr    chan error
}

func NewMPEGTSDemuxer(
	rsession *gortsplib.ServerSession,
	pathManager serverPathManager,
	pathConf *conf.Path,
	author defs.Publisher,
	l logger.Writer,
	packetsLost *counterdumper.Dumper,
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
		packetsLost:  packetsLost,
		decodeErrors: decodeErrors,
		pathName:     pathName,
		query:        query,
		ctx:          ctx,
		ctxCancel:    ctxCancel,
		initDone:     make(chan error, 1),
		loopErr:      make(chan error, 1),
	}
}

func (d *MPEGTSDemuxer) Start() {
	d.rtpReader = newRTPMPEGTSReader(d.packetsLost)

	// Find the MPEG-TS media and format
	medias := d.rsession.AnnouncedDescription().Medias
	if len(medias) != 1 || len(medias[0].Formats) != 1 {
		d.initDone <- errors.New("expected exactly one MPEG-TS track")
		return
	}
	medi := medias[0]
	forma := medi.Formats[0]

	d.rsession.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
		d.rtpReader.push(pkt)
	})

	go d.initializeAndRun()
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
		d.rtpReader.close()
		return nil, nil, errors.New("MPEG-TS initialization timeout: PMT not received")

	case <-d.ctx.Done():
		return nil, nil, errors.New("demuxer stopped")
	}
}

func (d *MPEGTSDemuxer) initializeAndRun() {
	err := d.initialize()
	d.initDone <- err
	if err != nil {
		return
	}

	err = d.runLoop()
	d.loopErr <- err
}

// initialize parses PAT/PMT and sets up the stream.
func (d *MPEGTSDemuxer) initialize() error {
	// Create the MPEG-TS enhanced reader
	d.tsReader = &mpegts.EnhancedReader{R: d.rtpReader}
	err := d.tsReader.Initialize()
	if err != nil {
		return fmt.Errorf("failed to initialize MPEG-TS reader: %w", err)
	}

	d.tsReader.OnDecodeError(func(err error) {
		d.decodeErrors.Add(err)
	})

	medias, err := mpegts.ToStream(d.tsReader, &d.subStream, d.logger)
	if err != nil {
		return fmt.Errorf("failed to map MPEG-TS to stream: %w", err)
	}

	d.logger.Log(logger.Info, "MPEG-TS demux discovered %d tracks", len(medias))

	d.path, d.subStream, err = d.pathManager.AddPublisher(defs.PathAddPublisherReq{
		Author:             d.author,
		Desc:               &description.Session{Medias: medias},
		UseRTPPackets:      false,
		ReplaceNTP:         true,
		ConfToCompare:      d.pathConf,
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

	return nil
}

func (d *MPEGTSDemuxer) runLoop() error {
	for {
		select {
		case <-d.ctx.Done():
			return nil
		default:
		}

		err := d.tsReader.Read()
		if err != nil {
			return err
		}
	}
}

func (d *MPEGTSDemuxer) Stop() {
	d.ctxCancel()
	d.rtpReader.close()
}

func (d *MPEGTSDemuxer) WaitForLoopError() error {
	select {
	case err := <-d.loopErr:
		return err
	case <-d.ctx.Done():
		return nil
	}
}

func IsMPEGTSFormat(forma format.Format) bool {
	return forma.Codec() == "MPEG-TS"
}
