// Package udp contains the UDP static source.
package udp

import (
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/multicast"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/counterdumper"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/mpegts"
	"github.com/bluenviron/mediamtx/internal/restrictnetwork"
	"github.com/bluenviron/mediamtx/internal/stream"
)

const (
	// same size as GStreamer's rtspsrc
	udpKernelReadBufferSize = 0x80000
)

type packetConnReader struct {
	pc       net.PacketConn
	sourceIP net.IP
}

func (r *packetConnReader) Read(p []byte) (int, error) {
	for {
		n, addr, err := r.pc.ReadFrom(p)

		if r.sourceIP != nil && addr != nil && !addr.(*net.UDPAddr).IP.Equal(r.sourceIP) {
			continue
		}

		return n, err
	}
}

type packetConn interface {
	net.PacketConn
	SetReadBuffer(int) error
}

// Source is a UDP static source.
type Source struct {
	ReadTimeout conf.Duration
	Parent      defs.StaticSourceParent
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...interface{}) {
	s.Parent.Log(level, "[UDP source] "+format, args...)
}

// Run implements StaticSource.
func (s *Source) Run(params defs.StaticSourceRunParams) error {
	s.Log(logger.Debug, "connecting")

	u, err := url.Parse(params.ResolvedSource)
	if err != nil {
		return err
	}
	q := u.Query()

	var sourceIP net.IP

	if src := q.Get("source"); src != "" {
		sourceIP = net.ParseIP(src)
		if sourceIP == nil {
			return fmt.Errorf("invalid source IP")
		}
	}

	addr, err := net.ResolveUDPAddr("udp", u.Host)
	if err != nil {
		return err
	}

	var pc packetConn

	if ip4 := addr.IP.To4(); ip4 != nil && addr.IP.IsMulticast() {
		if intfName := q.Get("interface"); intfName != "" {
			var intf *net.Interface
			intf, err = net.InterfaceByName(intfName)
			if err != nil {
				return err
			}

			pc, err = multicast.NewSingleConn(intf, addr.String(), net.ListenPacket)
			if err != nil {
				return err
			}
		} else {
			pc, err = multicast.NewMultiConn(addr.String(), true, net.ListenPacket)
			if err != nil {
				return err
			}
		}
	} else {
		var tmp net.PacketConn
		tmp, err = net.ListenPacket(restrictnetwork.Restrict("udp", addr.String()))
		if err != nil {
			return err
		}
		pc = tmp.(*net.UDPConn)
	}

	defer pc.Close()

	err = pc.SetReadBuffer(udpKernelReadBufferSize)
	if err != nil {
		return err
	}

	readerErr := make(chan error)
	go func() {
		readerErr <- s.runReader(pc, sourceIP)
	}()

	select {
	case err := <-readerErr:
		return err

	case <-params.Context.Done():
		pc.Close()
		<-readerErr
		return fmt.Errorf("terminated")
	}
}

func (s *Source) runReader(pc net.PacketConn, sourceIP net.IP) error {
	pc.SetReadDeadline(time.Now().Add(time.Duration(s.ReadTimeout)))
	pcr := &packetConnReader{pc: pc, sourceIP: sourceIP}
	r := &mpegts.EnhancedReader{R: pcr}
	err := r.Initialize()
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
		pc.SetReadDeadline(time.Now().Add(time.Duration(s.ReadTimeout)))
		err := r.Read()
		if err != nil {
			return err
		}
	}
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: "udpSource",
		ID:   "",
	}
}
