// Package mpegts contains the MPEG-TS static source.
package mpegts

import (
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/errordumper"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/packetdumper"
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
	DumpPackets       bool
	ReadTimeout       conf.Duration
	UDPReadBufferSize uint
	Parent            parent
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...any) {
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
		params := unix.URLToParams(u)
		l := &unix.Listener{
			Path: params.Path,
		}

		if s.DumpPackets {
			l.Listen = func(network, address string) (net.Listener, error) {
				ln, err2 := net.Listen(network, address)
				if err2 != nil {
					return nil, err2
				}

				return &packetdumper.Listener{
					Wrapped: ln,
					Prefix:  "mpegts_source_unix_conn",
				}, nil
			}
		}

		err = l.Initialize()
		if err != nil {
			return err
		}
		nc = l

	default:
		udpReadBufferSize := s.UDPReadBufferSize
		if params.Conf.MPEGTSUDPReadBufferSize != nil {
			udpReadBufferSize = *params.Conf.MPEGTSUDPReadBufferSize
		}

		params := udp.URLToParams(u)

		l := &udp.Listener{
			Address:           params.Address,
			Source:            params.Source,
			IntfName:          params.IntfName,
			UDPReadBufferSize: int(udpReadBufferSize),
		}

		l.ListenPacket = func(network, address string) (net.PacketConn, error) {
			pc, err2 := net.ListenPacket(network, address)
			if err2 != nil {
				return nil, err2
			}

			if s.DumpPackets {
				pc2 := &packetdumper.PacketConn{
					Wrapped: pc,
					Prefix:  "mpegts_source_packet_conn",
				}
				err2 = pc2.Initialize()
				if err2 != nil {
					pc.Close() //nolint:errcheck
					return nil, err2
				}

				pc = pc2
			}

			return pc, nil
		}

		err = l.Initialize()
		if err != nil {
			return err
		}
		nc = l
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

	mr.OnDecodeError(func(err error) {
		decodeErrors.Add(err)
	})

	var subStream *stream.SubStream

	medias, err := mpegts.ToStream(mr, &subStream, s)
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
		nc.SetReadDeadline(time.Now().Add(time.Duration(s.ReadTimeout)))
		err = mr.Read()
		if err != nil {
			return err
		}
	}
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() *defs.APIPathSource {
	return &defs.APIPathSource{
		Type: defs.APIPathSourceTypeMPEGTSSource,
		ID:   "",
	}
}
