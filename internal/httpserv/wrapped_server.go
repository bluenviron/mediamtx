// Package httpserv contains HTTP server utilities.
package httpserv

import (
	"context"
	"crypto/tls"
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
	ln    net.Listener
	inner *http.Server
}

// NewWrappedServer allocates a WrappedServer.
func NewWrappedServer(
	network string,
	address string,
	readTimeout time.Duration,
	serverCert string,
	serverKey string,
	handler http.Handler,
	parent logger.Writer,
) (*WrappedServer, error) {
	ln, err := net.Listen(network, address)
	if err != nil {
		return nil, err
	}

	var tlsConfig *tls.Config
	if serverCert != "" {
		crt, err := tls.LoadX509KeyPair(serverCert, serverKey)
		if err != nil {
			ln.Close()
			return nil, err
		}

		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{crt},
		}
	}

	h := handler
	h = &handlerFilterRequests{h}
	h = &handlerFilterRequests{h}
	h = &handlerServerHeader{h}
	h = &handlerLogger{h, parent}
	h = &handlerExitOnPanic{h}

	s := &WrappedServer{
		ln: ln,
		inner: &http.Server{
			Handler:           h,
			TLSConfig:         tlsConfig,
			ReadHeaderTimeout: readTimeout,
			ErrorLog:          log.New(&nilWriter{}, "", 0),
		},
	}

	if tlsConfig != nil {
		go s.inner.ServeTLS(s.ln, "", "")
	} else {
		go s.inner.Serve(s.ln)
	}

	return s, nil
}

// Close closes all resources and waits for all routines to return.
func (s *WrappedServer) Close() {
	s.inner.Shutdown(context.Background())
	s.ln.Close() // in case Shutdown() is called before Serve()
}
