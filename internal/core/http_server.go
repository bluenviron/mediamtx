package core

import (
	"context"
	"crypto/tls"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
)

type nilWriter struct{}

func (nilWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

type httpServer struct {
	ln    net.Listener
	inner *http.Server
}

func newHTTPServer(
	address string,
	readTimeout conf.StringDuration,
	serverCert string,
	serverKey string,
	handler http.Handler,
) (*httpServer, error) {
	ln, err := net.Listen(restrictNetwork("tcp", address))
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

	s := &httpServer{
		ln: ln,
		inner: &http.Server{
			Handler:           handler,
			TLSConfig:         tlsConfig,
			ReadHeaderTimeout: time.Duration(readTimeout),
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

func (s *httpServer) close() {
	s.inner.Shutdown(context.Background())
	s.ln.Close() // in case Shutdown() is called before Serve()
}
