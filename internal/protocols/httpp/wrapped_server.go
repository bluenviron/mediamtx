// Package httpp contains HTTP utilities.
package httpp

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/bluenviron/mediamtx/internal/logger"
)

type nilWriter struct{}

func (nilWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

// WrappedServer is a wrapper around http.Server that provides:
// - net.Listener allocation and closure
// - TLS allocation
// - exit on panic
// - logging
// - server header
// - filtering of invalid requests
type WrappedServer struct {
	Network     string
	Address     string
	ReadTimeout time.Duration
	Encryption  bool
	ServerCert  string
	ServerKey   string
	Handler     http.Handler
	Parent      logger.Writer

	ln    net.Listener
	inner *http.Server
}

// Initialize initializes a WrappedServer.
func (s *WrappedServer) Initialize() error {
	var tlsConfig *tls.Config
	if s.Encryption {
		if s.ServerCert == "" {
			return fmt.Errorf("server cert is missing")
		}
		crt, err := tls.LoadX509KeyPair(s.ServerCert, s.ServerKey)
		if err != nil {
			return err
		}

		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{crt},
		}
	}

	var err error
	s.ln, err = net.Listen(s.Network, s.Address)
	if err != nil {
		return err
	}

	h := s.Handler
	h = &handlerFilterRequests{h}
	h = &handlerFilterRequests{h}
	h = &handlerServerHeader{h}
	h = &handlerLogger{h, s.Parent}
	h = &handlerExitOnPanic{h}

	s.inner = &http.Server{
		Handler:           h,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: s.ReadTimeout,
		ErrorLog:          log.New(&nilWriter{}, "", 0),
	}

	if tlsConfig != nil {
		go s.inner.ServeTLS(s.ln, "", "")
	} else {
		go s.inner.Serve(s.ln)
	}

	return nil
}

// Close closes all resources and waits for all routines to return.
func (s *WrappedServer) Close() {
	ctx, ctxCancel := context.WithCancel(context.Background())
	ctxCancel()
	s.inner.Shutdown(ctx)
	s.ln.Close() // in case Shutdown() is called before Serve()
}
