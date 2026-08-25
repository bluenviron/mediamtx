package rtsp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/errordumper"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/mpegts"
	"github.com/bluenviron/mediamtx/internal/stream"
)

func findSingleMPEGTSFormat(desc *description.Session) (*description.Media, *format.MPEGTS) {
	if len(desc.Medias) != 1 || len(desc.Medias[0].Formats) != 1 {
		return nil, nil
	}

	forma, ok := desc.Medias[0].Formats[0].(*format.MPEGTS)
	if !ok {
		return nil, nil
	}

	return desc.Medias[0], forma
}

// mpegtsDemuxer demuxes an MPEG-TS stream received via RTP into component streams.
type mpegtsDemuxer struct {
	log          logger.Writer
	parent       parent
	client       *gortsplib.Client
	mpegtsMedia  *description.Media
	mpegtsFormat *format.MPEGTS
	decodeErrors *errordumper.Dumper

	pipeWriter *io.PipeWriter
	errChan    chan error
	closeOnce  sync.Once
}

func (d *mpegtsDemuxer) initialize() error {
	decoder, err := d.mpegtsFormat.CreateDecoder()
	if err != nil {
		return fmt.Errorf("failed to create MPEG-TS decoder: %w", err)
	}

	pr, pw := io.Pipe()
	d.pipeWriter = pw
	d.errChan = make(chan error, 1)

	d.client.OnPacketRTP(d.mpegtsMedia, d.mpegtsFormat, func(pkt *rtp.Packet) {
		tsData, decErr := decoder.Decode(pkt)
		if decErr != nil {
			d.decodeErrors.Add(decErr)
			return
		}

		for _, data := range tsData {
			_, writeErr := pw.Write(data)
			if writeErr != nil {
				d.log.Log(logger.Warn, "demuxer pipe write error: %v", writeErr)
				return
			}
		}
	})

	go d.run(pr)

	return nil
}

func (d *mpegtsDemuxer) close() {
	d.closeOnce.Do(func() {
		if d.pipeWriter != nil {
			d.pipeWriter.CloseWithError(io.EOF)
		}
	})
}

func (d *mpegtsDemuxer) wait(ctx context.Context) error {
	select {
	case err := <-d.errChan:
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err

	case <-ctx.Done():
		d.close()
		err := <-d.errChan
		if err == nil || errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
}

func (d *mpegtsDemuxer) run(pr *io.PipeReader) {
	err := d.doRun(pr)
	if err != nil && !errors.Is(err, io.EOF) {
		d.log.Log(logger.Error, "MPEG-TS demuxer error: %v", err)
	}
	d.errChan <- err
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

	medias, err := mpegts.ToStream(r, &subStream, d.log)
	if err != nil {
		return fmt.Errorf("failed to map MPEG-TS to stream: %w", err)
	}

	res := d.parent.SetReady(defs.PathSourceStaticSetReadyReq{
		Desc:          &description.Session{Medias: medias},
		UseRTPPackets: false,
		ReplaceNTP:    true,
	})
	if res.Err != nil {
		return res.Err
	}

	defer d.parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})

	subStream = res.SubStream

	for {
		err = r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}
