package moq

import (
	"context"
	"crypto/tls"
	"net"
	"os"
	"strings"

	"github.com/bluenviron/gortsplib/v5/pkg/readbuffer"
	"github.com/quic-go/quic-go"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/moq"
)

var supportedMoqtALPNs = []string{
	string(defs.APIMoQVersionDraft19),
	string(defs.APIMoQVersionDraft18),
	string(defs.APIMoQVersionDraft17),
	string(defs.APIMoQVersionDraft16),
}

type nativeListenerParent interface {
	newSession(req newSessionReq) newSessionRes
	Log(level logger.Level, format string, args ...any)
}

type nativeListener struct {
	address           string
	getCertificate    func(*tls.ClientHelloInfo) (*tls.Certificate, error)
	udpReadBufferSize uint
	parent            nativeListenerParent

	ctx       context.Context
	ctxCancel context.CancelFunc
	ln        net.PacketConn
	listener  *quic.Listener
	done      chan struct{}
}

func (s *nativeListener) initialize() error {
	ctx, ctxCancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.ctxCancel = ctxCancel
	s.done = make(chan struct{})

	os.Setenv("QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING", "true") //nolint:errcheck

	ln, err := net.ListenPacket("udp", s.address)
	if err != nil {
		ctxCancel()
		return err
	}
	s.ln = ln

	if s.udpReadBufferSize != 0 {
		err = readbuffer.SetReadBuffer(s.ln.(*net.UDPConn), int(s.udpReadBufferSize))
		if err != nil {
			s.ln.Close()
			ctxCancel()
			return err
		}
	}

	tlsConfig := &tls.Config{
		GetCertificate: s.getCertificate,
		NextProtos:     supportedMoqtALPNs,
	}

	listener, err := quic.Listen(s.ln, tlsConfig, &quic.Config{
		EnableDatagrams: true,
	})
	if err != nil {
		s.ln.Close()
		ctxCancel()
		return err
	}
	s.listener = listener

	go s.run()

	return nil
}

func alpnToVersion(alpn string) defs.APIMoQVersion {
	switch alpn {
	case string(defs.APIMoQVersionDraft19):
		return defs.APIMoQVersionDraft19

	case string(defs.APIMoQVersionDraft18):
		return defs.APIMoQVersionDraft18

	case string(defs.APIMoQVersionDraft17):
		return defs.APIMoQVersionDraft17

	case string(defs.APIMoQVersionDraft16):
		return defs.APIMoQVersionDraft16
	}

	return ""
}

func (s *nativeListener) run() {
	defer close(s.done)

	for {
		conn, err := s.listener.Accept(s.ctx)
		if err != nil {
			if s.ctx.Err() != nil || strings.Contains(err.Error(), "closed") {
				return
			}
			s.parent.Log(logger.Warn, "[MoQ] native QUIC accept error: %v", err)
			continue
		}

		version := alpnToVersion(conn.ConnectionState().TLS.NegotiatedProtocol)
		if version == "" {
			conn.CloseWithError(0, "unsupported ALPN") //nolint:errcheck
			continue
		}

		res := s.parent.newSession(newSessionReq{
			version: version,
			conn: &moq.ConnQUIC{
				Conn: conn,
			},
		})
		if res.err != nil {
			conn.CloseWithError(0, res.err.Error()) //nolint:errcheck
		}
	}
}

func (s *nativeListener) close() {
	s.ctxCancel()
	if s.listener != nil {
		s.listener.Close() //nolint:errcheck
	}
	if s.ln != nil {
		s.ln.Close()
	}
	<-s.done
}
