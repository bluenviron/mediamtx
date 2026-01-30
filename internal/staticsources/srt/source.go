// Package srt contains the SRT static source.
package srt

import (
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	srt "github.com/datarhei/gosrt"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/errordumper"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/mpegts"
	"github.com/bluenviron/mediamtx/internal/stream"
)

type parent interface {
	logger.Writer
	SetReady(req defs.PathSourceStaticSetReadyReq) defs.PathSourceStaticSetReadyRes
	SetNotReady(req defs.PathSourceStaticSetNotReadyReq)
}

// Source is a SRT static source.
type Source struct {
	ReadTimeout conf.Duration
	Parent      parent
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...any) {
	s.Parent.Log(level, "[SRT source] "+format, args...)
}

// Run implements StaticSource.
func (s *Source) Run(params defs.StaticSourceRunParams) error {
	s.Log(logger.Debug, "connecting")

	conf := srt.DefaultConfig()
	address, err := conf.UnmarshalURL(params.ResolvedSource)
	if err != nil {
		return err
	}

	err = conf.Validate()
	if err != nil {
		return err
	}

	sconn, err := srt.Dial("srt", address, conf)
	if err != nil {
		return err
	}

	readDone := make(chan error)
	go func() {
		readDone <- s.runReader(sconn)
	}()

	for {
		select {
		case err = <-readDone:
			sconn.Close()
			return err

		case <-params.ReloadConf:

		case <-params.Context.Done():
			sconn.Close()
			<-readDone
			return nil
		}
	}
}

func (s *Source) runReader(sconn srt.Conn) error {
	sconn.SetReadDeadline(time.Now().Add(time.Duration(s.ReadTimeout)))
	r := &mpegts.EnhancedReader{R: sconn}
	err := r.Initialize()
	if err != nil {
		return err
	}

	decodeErrors := &errordumper.Dumper{
		OnReport: func(val uint64, last error) {
			if val == 1 {
				s.Log(logger.Warn, "decode error: %v", last)
			} else {
				s.Log(logger.Warn, "%d decode errors, last was: %v", val, last)
			}
		},
	}

	decodeErrors.Start()
	defer decodeErrors.Stop()

	r.OnDecodeError(func(err error) {
		decodeErrors.Add(err)
	})

	var subStream *stream.SubStream

	medias, err := mpegts.ToStream(r, &subStream, s)
	if err != nil {
		return err
	}

	res := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
		Desc:          &description.Session{Medias: medias},
		UseRTPPackets: false,
		ReplaceNTP:    true,
	})
	if res.Err != nil {
		return res.Err
	}

	defer s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})

	subStream = res.SubStream

	for {
		sconn.SetReadDeadline(time.Now().Add(time.Duration(s.ReadTimeout)))
		err = r.Read()
		if err != nil {
			return err
		}
	}
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() *defs.APIPathSource {
	return &defs.APIPathSource{
		Type: "srtSource",
		ID:   "",
	}
}
