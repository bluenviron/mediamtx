package httpp_test

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestHTTPS2(t *testing.T) {
	cert, err := tls.X509KeyPair(test.TLSCertPub, test.TLSCertKey)
	require.NoError(t, err)

	s := &httpp.Server{
		Address:      "127.0.0.1:4555",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		Encryption:   true,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return &cert, nil
		},
		Parent: test.NilLogger,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}
	err = s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	client := &http.Client{Transport: &http.Transport{
		ForceAttemptHTTP2: true,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test server uses a dummy certificate
	}}
	defer client.CloseIdleConnections()

	res, err := client.Get("https://127.0.0.1:4555/")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, 2, res.ProtoMajor)
}

func TestUnixSocket(t *testing.T) {
	s := &httpp.Server{
		Address:      "unix://http.sock",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		Parent:       test.NilLogger,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}
	err := s.Initialize()
	require.NoError(t, err)

	info, err := os.Stat("http.sock")
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o755), info.Mode().Perm())

	conn, err := net.Dial("unix", "http.sock")
	require.NoError(t, err)

	_, err = conn.Write([]byte("OPTIONS / HTTP/1.1\n" +
		"Host: localhost:8889\n\n"))
	require.NoError(t, err)

	buf := make([]byte, 200)
	n, err := conn.Read(buf)
	require.NoError(t, err)

	res := strings.Split(string(buf[:n]), "\r\n")
	require.Equal(t, "HTTP/1.1 200 OK", res[0])

	conn.Close()
	s.Close()

	_, err = os.Stat("http.sock")
	require.EqualError(t, err, "stat http.sock: no such file or directory")
}

// A handler that flushes mid-response must have its bytes reach the client
// before it returns. Both wrappers between the http.Server and the handler
// (the logger's response recorder and the write-timeout
// writer) sit in that path, so a missing Flusher on either one
// silently withholds the output until the handler is done — which defeats
// server-sent events and any other long response.
func TestResponseWriterWrappersPropagateFlush(t *testing.T) {
	flushed := make(chan struct{})
	release := make(chan struct{})

	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		f, ok := w.(http.Flusher)
		require.True(t, ok, "handler must see a Flusher")

		_, err := w.Write([]byte("data: first\n\n"))
		require.NoError(t, err)
		f.Flush()

		close(flushed)
		<-release
	})

	s := &httpp.Server{
		Address:      "127.0.0.1:4556",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		Parent:       test.NilLogger,
		Handler:      h,
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	conn, err := net.Dial("tcp", "127.0.0.1:4556")
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Write([]byte("GET / HTTP/1.1\r\nHost: localhost\r\n\r\n"))
	require.NoError(t, err)

	<-flushed

	// The handler is still blocked on `release`, so anything that arrives now
	// arrived because of the Flush, not because the response was closed. Without
	// the flush propagating, nothing arrives and the deadline below fires.
	err = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	require.NoError(t, err)

	br := bufio.NewReader(conn)

	// Skip the status line and headers.
	for {
		line, err2 := br.ReadString('\n')
		require.NoError(t, err2)
		if line == "\r\n" {
			break
		}
	}

	// No Content-Length, so the body is chunked: accumulate until the payload
	// shows up, since a single Read may return just the chunk size line.
	var got []byte
	buf := make([]byte, 256)
	for !bytes.Contains(got, []byte("data: first")) {
		n, err2 := br.Read(buf)
		require.NoError(t, err2)
		got = append(got, buf[:n]...)
	}

	close(release)
}
