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

	"github.com/bluenviron/mediamtx/internal/errordumper"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/mpegts"
	"github.com/bluenviron/mediamtx/internal/stream"
)

// MPEGTSDemuxerSource is a source that provides MPEG-TS RTP packets.
type MPEGTSDemuxerSource interface {
	OnPacketRTP(*description.Media, format.Format, gortsplib.OnPacketRTPFunc)
}

// MPEGTSDemuxerOnTracksFunc is called when MPEG-TS tracks have been detected.
type MPEGTSDemuxerOnTracksFunc func(*description.Session) (*stream.SubStream, error)

// FindSingleMPEGTSFormat returns the single MPEG-TS format in a description, if present.
func FindSingleMPEGTSFormat(desc *description.Session) (*description.Media, *format.MPEGTS) {
	if len(desc.Medias) != 1 || len(desc.Medias[0].Formats) != 1 {
		return nil, nil
	}

	forma, ok := desc.Medias[0].Formats[0].(*format.MPEGTS)
	if !ok {
		return nil, nil
	}

	return desc.Medias[0], forma
}

// MPEGTSDemuxer demuxes an MPEG-TS stream received via RTP into component streams.
type MPEGTSDemuxer struct {
	Source       MPEGTSDemuxerSource
	Log          logger.Writer
	Media        *description.Media
	Format       *format.MPEGTS
	DecodeErrors *errordumper.Dumper
	OnTracks     MPEGTSDemuxerOnTracksFunc
	OnError      func(error)

	pipeWriter *io.PipeWriter
	errChan    chan error
	closeOnce  sync.Once
}

// Initialize initializes the demuxer.
func (d *MPEGTSDemuxer) Initialize() error {
	decoder, err := d.Format.CreateDecoder()
	if err != nil {
		return fmt.Errorf("failed to create MPEG-TS decoder: %w", err)
	}

	pr, pw := io.Pipe()
	d.pipeWriter = pw
	d.errChan = make(chan error, 1)

	d.Source.OnPacketRTP(d.Media, d.Format, func(pkt *rtp.Packet) {
		tsData, decErr := decoder.Decode(pkt)
		if decErr != nil {
			d.DecodeErrors.Add(decErr)
			return
		}

		for _, data := range tsData {
			_, writeErr := pw.Write(data)
			if writeErr != nil {
				d.Log.Log(logger.Warn, "demuxer pipe write error: %v", writeErr)
				return
			}
		}
	})

	go d.run(pr)

	return nil
}

// Close closes the demuxer.
func (d *MPEGTSDemuxer) Close() {
	d.closeOnce.Do(func() {
		if d.pipeWriter != nil {
			d.pipeWriter.CloseWithError(io.EOF)
		}
	})
}

// Wait waits until the demuxer exits.
func (d *MPEGTSDemuxer) Wait(ctx context.Context) error {
	select {
	case err := <-d.errChan:
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err

	case <-ctx.Done():
		d.Close()
		err := <-d.errChan
		if err == nil || errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
}

func (d *MPEGTSDemuxer) run(pr *io.PipeReader) {
	err := d.doRun(pr)
	if err != nil && !errors.Is(err, io.EOF) {
		d.Log.Log(logger.Error, "MPEG-TS demuxer error: %v", err)
		if d.OnError != nil {
			d.OnError(err)
		}
	}
	d.errChan <- err
}

func (d *MPEGTSDemuxer) doRun(pr *io.PipeReader) error {
	r := &mpegts.EnhancedReader{R: pr}
	err := r.Initialize()
	if err != nil {
		return fmt.Errorf("failed to initialize MPEG-TS reader: %w", err)
	}

	r.OnDecodeError(func(err error) {
		d.DecodeErrors.Add(err)
	})

	var subStream *stream.SubStream

	medias, err := mpegts.ToStream(r, &subStream, d.Log)
	if err != nil {
		return fmt.Errorf("failed to map MPEG-TS to stream: %w", err)
	}

	subStream, err = d.OnTracks(&description.Session{Medias: medias})
	if err != nil {
		return err
	}

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
