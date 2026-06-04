// Package httpp3 contains HTTP/3 utilities.
package httpp3

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/readbuffer"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
)

const (
	// WebTransport requires certificates valid for at most 14 days.
	certValidity = 14 * 24 * time.Hour
	certRotation = 13 * 24 * time.Hour
)

func generateWebTransportCert() (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}

	now := time.Now()
	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "mediamtx"},
		NotBefore:    now,
		NotAfter:     now.Add(certValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	return &cert, nil
}

// Server is a wrapper around http3.Server that provides:
// - net.PacketConn creation and destruction
// - JIT TLS certificate generation and rotation (WebTransport-compatible: ECDSA P-256, 14-day validity)
type Server struct {
	Address            string
	UDPReadBufferSize  uint
	EnableWebTransport bool
	Handler            http.Handler
	Parent             logger.Writer

	ln        net.PacketConn
	inner     *http3.Server
	wtServer  *webtransport.Server
	cert      *tls.Certificate
	certMu    sync.RWMutex
	terminate chan struct{}
}

// Initialize initializes a Server.
func (s *Server) Initialize() error {
	os.Setenv("QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING", "true") //nolint:errcheck

	s.terminate = make(chan struct{})

	var err error
	s.ln, err = net.ListenPacket("udp", s.Address)
	if err != nil {
		return err
	}

	if s.UDPReadBufferSize != 0 {
		err = readbuffer.SetReadBuffer(s.ln.(*net.UDPConn), int(s.UDPReadBufferSize))
		if err != nil {
			s.ln.Close()
			return err
		}
	}

	cert, err := generateWebTransportCert()
	if err != nil {
		return err
	}
	s.cert = cert

	tlsConfig := &tls.Config{
		GetCertificate: func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
			s.certMu.RLock()
			defer s.certMu.RUnlock()
			return s.cert, nil
		},
	}

	s.inner = &http3.Server{
		Handler:   s.Handler,
		TLSConfig: tlsConfig,
	}

	if s.EnableWebTransport {
		// webtransport.Server.Serve calls quic.ListenEarly directly with s.H3.TLSConfig,
		// bypassing the http3.ConfigureTLSConfig call that http3.Server.Serve does internally.
		// Pre-configure to set NextProtos = ["h3"] so QUIC ALPN negotiation succeeds.
		s.inner.TLSConfig = http3.ConfigureTLSConfig(tlsConfig)
		webtransport.ConfigureHTTP3Server(s.inner)
		s.wtServer = &webtransport.Server{
			H3:          s.inner,
			CheckOrigin: func(_ *http.Request) bool { return true },
		}
		go s.wtServer.Serve(s.ln) //nolint:errcheck
	} else {
		go s.inner.Serve(s.ln) //nolint:errcheck
	}

	go s.rotateCert()

	return nil
}

func (s *Server) rotateCert() {
	t := time.NewTimer(certRotation)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			cert, err := generateWebTransportCert()
			if err != nil {
				s.Parent.Log(logger.Error, "failed to rotate WebTransport certificate: %v", err)
			} else {
				s.certMu.Lock()
				s.cert = cert
				s.certMu.Unlock()
				s.Parent.Log(logger.Info, "WebTransport certificate rotated")
			}
			t.Reset(certRotation)

		case <-s.terminate:
			return
		}
	}
}

// Close closes all resources and waits for all routines to return.
func (s *Server) Close() {
	close(s.terminate)
	s.ln.Close()
	if s.wtServer != nil {
		s.wtServer.Close() //nolint:errcheck
	} else {
		s.inner.Close() //nolint:errcheck
	}
}

// Certificate returns the current TLS certificate.
func (s *Server) Certificate() *tls.Certificate {
	s.certMu.RLock()
	defer s.certMu.RUnlock()
	return s.cert
}

// Upgrade upgrades an HTTP/3 request to a WebTransport session.
// Only valid when EnableWebTransport is true.
func (s *Server) Upgrade(w http.ResponseWriter, r *http.Request) (*webtransport.Session, error) {
	return s.wtServer.Upgrade(w, r)
}
