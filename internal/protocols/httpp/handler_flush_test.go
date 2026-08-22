package httpp

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/test"
)

// A handler that flushes mid-response must have its bytes reach the client
// before it returns. Both wrappers between the http.Server and the handler
// (handlerLogger's responseRecorder and handlerWriteTimeout's
// writeTimeoutWriter) sit in that path, so a missing Flusher on either one
// silently withholds the output until the handler is done — which defeats
// server-sent events and any other long response.
func TestResponseWriterWrappersPropagateFlush(t *testing.T) {
	flushed := make(chan struct{})
	release := make(chan struct{})

	var h http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

	h = &handlerLogger{h, test.NilLogger}
	h = &handlerWriteTimeout{h, 10 * time.Second}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	s := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	go s.Serve(ln)
	defer s.Shutdown(context.Background())

	conn, err := net.Dial("tcp", ln.Addr().String())
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
