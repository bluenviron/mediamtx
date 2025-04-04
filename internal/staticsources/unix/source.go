// Package unix contains the UNIX static source.
package unix

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	mcmpegts "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/counterdumper"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/mpegts"
	"github.com/bluenviron/mediamtx/internal/stream"
)

// Source is a unix static source.
type Source struct {
	ReadTimeout conf.Duration
	Parent      defs.StaticSourceParent
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...interface{}) {
	s.Parent.Log(level, "[unix source] "+format, args...)
}

// Run implements StaticSource.
func (s *Source) Run(params defs.StaticSourceRunParams) error {
	s.Log(logger.Debug, "connecting")

	network, address, err := net.SplitHostPort(params.ResolvedSource)
	if err != nil {
		return err
	}

	err = os.Remove(address)
	if err != nil {
		// not really important if it fails
		s.Log(logger.Debug, "Failed to remove previous unix socket", err)
	}

	var socket net.Listener
	socket, err = net.Listen(network, address)
	if err != nil {
		return err
	}
	defer socket.Close()

	conn, err := socket.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()

	readerErr := make(chan error)
	go func() {
		readerErr <- s.runReader(conn)
	}()

	select {
	case err := <-readerErr:
		return err

	case <-params.Context.Done():
		socket.Close()
		<-readerErr
		return fmt.Errorf("terminated")
	}
}

func (s *Source) runReader(conn net.Conn) error {
	conn.SetReadDeadline(time.Now().Add(time.Duration(s.ReadTimeout)))
	r, err := mcmpegts.NewReader(conn)
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

	r.OnDecodeError(func(_ error) {
		decodeErrors.Increase()
	})

	var stream *stream.Stream

	medias, err := mpegts.ToStream(r, &stream, s)
	if err != nil {
		return err
	}

	res := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
		Desc:               &description.Session{Medias: medias},
		GenerateRTPPackets: true,
	})
	if res.Err != nil {
		return res.Err
	}

	defer s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})

	stream = res.Stream

	for {
		conn.SetReadDeadline(time.Now().Add(time.Duration(s.ReadTimeout)))
		err := r.Read()
		if err != nil {
			return err
		}
	}
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: "unixSource",
		ID:   "",
	}
}
