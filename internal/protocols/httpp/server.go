// Package httpp contains HTTP utilities.
package httpp

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/certloader"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/restrictnetwork"
)

type nilWriter struct{}

func (nilWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

// Server is a wrapper around http.Server that provides:
// - net.Listener allocation and closure
// - TLS allocation
// - exit on panic
// - logging
// - server header
// - filtering of invalid requests
type Server struct {
	Address      string
	AllowOrigins []string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	Encryption   bool
	ServerCert   string
	ServerKey    string
	Handler      http.Handler
	Parent       logger.Writer

	ln      net.Listener
	inner   *http.Server
	loader  *certloader.CertLoader
	tracker *handlerTracker
}

// Initialize initializes a Server.
func (s *Server) Initialize() error {
	if s.ReadTimeout == 0 {
		return fmt.Errorf("invalid ReadTimeout")
	}
	if s.WriteTimeout == 0 {
		return fmt.Errorf("invalid WriteTimeout")
	}

	var tlsConfig *tls.Config
	if s.Encryption {
		if s.ServerCert == "" {
			return fmt.Errorf("server cert is missing")
		}

		s.loader = &certloader.CertLoader{
			CertPath: s.ServerCert,
			KeyPath:  s.ServerKey,
			Parent:   s.Parent,
		}
		err := s.loader.Initialize()
		if err != nil {
			return err
		}

		tlsConfig = &tls.Config{
			GetCertificate: s.loader.GetCertificate(),
		}
	}

	var network string
	var address string

	if strings.HasPrefix(s.Address, "unix://") {
		network = "unix"
		address = s.Address[len("unix://"):]
	} else {
		network, address = restrictnetwork.Restrict("tcp", s.Address)
	}

	if network == "unix" {
		os.Remove(address)
	}

	var err error
	s.ln, err = net.Listen(network, address)
	if err != nil {
		return err
	}

	if network == "unix" {
		os.Chmod(address, 0o755) //nolint:errcheck
	}

	h := s.Handler
	h = &handlerOrigin{h, s.AllowOrigins}
	h = &handlerServerHeader{h}
	h = &handlerFilterRequests{h}
	h = &handlerLogger{h, s.Parent}
	h = &handlerExitOnPanic{h}
	h = &handlerWriteTimeout{h, s.WriteTimeout}
	s.tracker = &handlerTracker{h: h}
	h = s.tracker

	s.inner = &http.Server{
		Handler:   h,
		TLSConfig: tlsConfig,

		// applied before reading any request
		ReadTimeout: s.ReadTimeout,

		// applied after HTTP handler has returned
		IdleTimeout: 30 * time.Second,

		ErrorLog: log.New(&nilWriter{}, "", 0),
	}

	if tlsConfig != nil {
		go s.inner.ServeTLS(s.ln, "", "")
	} else {
		go s.inner.Serve(s.ln)
	}

	return nil
}

// Close closes all resources and waits for all routines to return.
func (s *Server) Close() {
	s.ln.Close()
	s.inner.Close() //nolint:errcheck
	s.tracker.close()

	if s.loader != nil {
		s.loader.Close()
	}
}
