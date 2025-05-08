package httpp

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/src/test"
)

func TestFilterEmptyPath(t *testing.T) {
	s := &Server{
		Network:     "tcp",
		Address:     "localhost:4555",
		ReadTimeout: 10 * time.Second,
		Parent:      test.NilLogger,
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	conn, err := net.Dial("tcp", "localhost:4555")
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Write([]byte("OPTIONS http://localhost HTTP/1.1\n" +
		"Host: localhost:8889\n" +
		"Accept-Encoding: gzip\n" +
		"User-Agent: Go-http-client/1.1\n\n"))
	require.NoError(t, err)

	buf := make([]byte, 20)
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)
}
