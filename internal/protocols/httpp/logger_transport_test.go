package httpp_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
)

type testLogger struct {
	mutex sync.Mutex
	lines []string
}

func (l *testLogger) Log(_ logger.Level, format string, args ...any) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.lines = append(l.lines, strings.TrimSpace(fmt.Sprintf(format, args...)))
}

func TestLoggerTransport(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, "offer", string(body))
		w.Header().Set("Content-Type", "application/sdp")
		w.Header().Set("Set-Cookie", "secret")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("answer")) //nolint:errcheck
	}))
	defer s.Close()

	l := &testLogger{}
	c := &http.Client{Transport: &httpp.LoggerTransport{Transport: http.DefaultTransport, Log: l}}

	req, err := http.NewRequest(http.MethodPost, s.URL+"/test", strings.NewReader("offer"))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer secret")

	res, err := c.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equal(t, "answer", string(body))

	require.Len(t, l.lines, 2)
	require.Contains(t, l.lines[0], "[c->s] POST /test HTTP/1.1")
	require.Contains(t, l.lines[0], "Authorization: <redacted>")
	require.NotContains(t, l.lines[0], "secret")
	require.Contains(t, l.lines[0], "offer")
	require.Contains(t, l.lines[1], "[s->c] HTTP/1.1 201 Created")
	require.Contains(t, l.lines[1], "Set-Cookie: <redacted>")
	require.Contains(t, l.lines[1], "answer")
}

func TestLoggerTransportNoBody(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(make([]byte, 100)) //nolint:errcheck
	}))
	defer s.Close()

	l := &testLogger{}
	c := &http.Client{Transport: &httpp.LoggerTransport{Transport: http.DefaultTransport, Log: l}}

	res, err := c.Get(s.URL)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Len(t, l.lines, 2)
	require.Contains(t, l.lines[0], "[c->s] GET / HTTP/1.1")
	require.Contains(t, l.lines[1], "(body of 100 bytes)")
}
