package httpp_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
)

func TestServerRedactsResponseHeaders(t *testing.T) {
	l := &testLogger{}

	s := &httpp.Server{
		Address:      "127.0.0.1:4556",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		Parent:       l,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Set-Cookie", "session=secret")
			w.Header().Set("X-Api-Key", "secret-key")
			w.Header().Set("X-Other", "visible")
			w.WriteHeader(http.StatusOK)
		}),
	}
	require.NoError(t, s.Initialize())
	defer s.Close()

	res, err := http.Get("http://127.0.0.1:4556/")
	require.NoError(t, err)
	res.Body.Close()

	l.mutex.Lock()
	defer l.mutex.Unlock()

	var out string
	for _, line := range l.lines {
		if strings.Contains(line, "[s->c]") {
			out = line
		}
	}
	require.Contains(t, out, "Set-Cookie: <redacted>")
	require.Contains(t, out, "X-Api-Key: <redacted>")
	require.Contains(t, out, "X-Other: visible")
	require.NotContains(t, out, "secret")
}
