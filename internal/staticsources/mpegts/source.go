// Package mpegts contains the MPEG-TS static source.
package mpegts

import (
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/counterdumper"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/mpegts"
	"github.com/bluenviron/mediamtx/internal/protocols/udp"
	"github.com/bluenviron/mediamtx/internal/protocols/unix"
	"github.com/bluenviron/mediamtx/internal/stream"
)

type parent interface {
	logger.Writer
	SetReady(req defs.PathSourceStaticSetReadyReq) defs.PathSourceStaticSetReadyRes
	SetNotReady(req defs.PathSourceStaticSetNotReadyReq)
}

// Source is a MPEG-TS static source.
type Source struct {
	ReadTimeout conf.Duration
	Parent      parent
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...interface{}) {
	s.Parent.Log(level, "[MPEG-TS source] "+format, args...)
}

// Run implements StaticSource.
func (s *Source) Run(params defs.StaticSourceRunParams) error {
	s.Log(logger.Debug, "connecting")

	u, err := url.Parse(params.ResolvedSource)
	if err != nil {
		return err
	}

	var nc net.Conn

	switch u.Scheme {
	case "unix+mpegts":
		nc, err = unix.CreateConn(u)
		if err != nil {
			return err
		}

	default:
		nc, err = udp.CreateConn(u, int(params.Conf.MPEGTSUDPReadBufferSize))
		if err != nil {
			return err
		}
	}

	readerErr := make(chan error)
	go func() {
		readerErr <- s.runReader(nc)
	}()

	for {
		select {
		case err = <-readerErr:
			nc.Close()
			return err

		case <-params.ReloadConf:

		case <-params.Context.Done():
			nc.Close()
			<-readerErr
			return fmt.Errorf("terminated")
		}
	}
}

func (s *Source) runReader(nc net.Conn) error {
	nc.SetReadDeadline(time.Now().Add(time.Duration(s.ReadTimeout)))
	mr := &mpegts.EnhancedReader{R: nc}
	err := mr.Initialize()
	if err != nil {
		return err
	}

	decodeErrors := &counterdumper.CounterDumper{
		OnReport: func(val uint64) {
			s.Log(logger.Warn, "%d decode %s",
				val,
				func() string {
					if val == 1 {
						return "error"
					}
					return "errors"
				}())
		},
	}

	decodeErrors.Start()
	defer decodeErrors.Stop()

	mr.OnDecodeError(func(_ error) {
		decodeErrors.Increase()
	})

	var stream *stream.Stream

	medias, err := mpegts.ToStream(mr, &stream, s)
	if err != nil {
		return err
	}

	res := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
		Desc:               &description.Session{Medias: medias},
		GenerateRTPPackets: true,
		FillNTP:            true,
	})
	if res.Err != nil {
		return res.Err
	}

	defer s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})

	stream = res.Stream

	for {
		nc.SetReadDeadline(time.Now().Add(time.Duration(s.ReadTimeout)))
		err = mr.Read()
		if err != nil {
			return err
		}
	}
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: "mpegtsSource",
		ID:   "",
	}
}
