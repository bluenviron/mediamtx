package httpp

import (
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/test"
)

func TestFilterEmptyPath(t *testing.T) {
	s := &Server{
		Address:      "localhost:4555",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		Parent:       test.NilLogger,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	conn, err := net.Dial("tcp", "localhost:4555")
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Write([]byte("OPTIONS / HTTP/1.1\n" +
		"Host: localhost:8889\n\n"))
	require.NoError(t, err)

	buf := make([]byte, 200)
	n, err := conn.Read(buf)
	require.NoError(t, err)

	res := strings.Split(string(buf[:n]), "\r\n")
	require.Equal(t, "HTTP/1.1 200 OK", res[0])
}

func TestUnixSocket(t *testing.T) {
	s := &Server{
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

	_, err = os.Stat("http.sock")
	require.NoError(t, err)

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
