package rtsp

import (
	"errors"
	"fmt"
	"io"

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

// mpegtsDemuxer demuxes an MPEG-TS stream received via RTP into component streams.
type mpegtsDemuxer struct {
	session      *session
	pathManager  serverPathManager
	pathConf     *conf.Path
	mpegtsMedia  *description.Media
	mpegtsFormat *format.MPEGTS
	decodeErrors *errordumper.Dumper
	pathName     string
	query        string

	pipeWriter *io.PipeWriter
}

func (d *mpegtsDemuxer) initialize() error {
	decoder, err := d.mpegtsFormat.CreateDecoder()
	if err != nil {
		return fmt.Errorf("failed to create MPEG-TS decoder: %w", err)
	}

	pr, pw := io.Pipe()
	d.pipeWriter = pw

	d.session.rsession.OnPacketRTP(d.mpegtsMedia, d.mpegtsFormat, func(pkt *rtp.Packet) {
		tsData, decErr := decoder.Decode(pkt)
		if decErr != nil {
			d.decodeErrors.Add(decErr)
			return
		}

		for _, data := range tsData {
			_, err = pw.Write(data)
			if err != nil {
				d.session.Log(logger.Warn, "demuxer pipe write error: %v", err)
				return
			}
		}
	})

	go d.run(pr)

	return nil
}

func (d *mpegtsDemuxer) close() {
	d.pipeWriter.CloseWithError(io.EOF)
}

func (d *mpegtsDemuxer) run(pr *io.PipeReader) {
	err := d.doRun(pr)
	if err != nil {
		d.session.Log(logger.Error, "MPEG-TS demuxer error: %v", err)
		d.session.Close()
	}
}

func (d *mpegtsDemuxer) doRun(pr *io.PipeReader) error {
	r := &mpegts.EnhancedReader{R: pr}
	err := r.Initialize()
	if err != nil {
		return fmt.Errorf("failed to initialize MPEG-TS reader: %w", err)
	}

	r.OnDecodeError(func(err error) {
		d.decodeErrors.Add(err)
	})

	var subStream *stream.SubStream

	medias, err := mpegts.ToStream(r, &subStream, d.session)
	if err != nil {
		return fmt.Errorf("failed to map MPEG-TS to stream: %w", err)
	}

	res, err := d.pathManager.AddPublisher(defs.PathAddPublisherReq{
		Author:        d.session,
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

	defer res.Path.RemovePublisher(defs.PathRemovePublisherReq{Author: d.session})

	subStream = res.SubStream

	for {
		err = r.Read()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return err
			}
			return nil
		}
	}
}
