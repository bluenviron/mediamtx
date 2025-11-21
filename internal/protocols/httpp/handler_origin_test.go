package httpp

import (
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestHandlerOrigin(t *testing.T) {
	for _, ca := range []struct {
		name           string
		origin         string
		allowedOrigins []string
		expected       string
	}{
		{
			"empty",
			"",
			[]string{},
			"",
		},
		{
			"not allowed",
			"http://another.com",
			[]string{"http://example.com"},
			"",
		},
		{
			"everything allowed, no origin",
			"",
			[]string{"*"},
			"*",
		},
		{
			"everything allowed, with origin",
			"https://example.com",
			[]string{"*"},
			"*",
		},
		{
			"allowed",
			"https://example.org",
			[]string{"http://example.com", "https://example.org"},
			"https://example.org",
		},
		{
			"wildcard",
			"https://test.example.org",
			[]string{"https://*.example.org"},
			"https://test.example.org",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			s := &Server{
				Address:      "localhost:4555",
				AllowOrigins: ca.allowedOrigins,
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

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr}

			req, err := http.NewRequest(http.MethodGet, "http://localhost:4555", nil)
			require.NoError(t, err)

			req.Header.Set("Origin", ca.origin)

			res, err := hc.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			require.Equal(t, ca.expected, res.Header.Get("Access-Control-Allow-Origin"))
		})
	}
}
