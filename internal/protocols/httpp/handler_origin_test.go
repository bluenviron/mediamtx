package httpp_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestHandlerOriginAddAllowOriginHeader(t *testing.T) {
	for _, ca := range []struct {
		name           string
		origin         string
		allowedOrigins []string
		expected       string
	}{
		{
			name:           "empty",
			allowedOrigins: []string{},
		},
		{
			name:           "not allowed",
			origin:         "http://another.com",
			allowedOrigins: []string{"http://example.com"},
		},
		{
			name:           "everything allowed, no origin",
			allowedOrigins: []string{"*"},
		},
		{
			name:           "everything allowed, with origin",
			origin:         "https://example.com",
			allowedOrigins: []string{"*"},
			expected:       "https://example.com",
		},
		{
			name:           "allowed",
			origin:         "https://example.org",
			allowedOrigins: []string{"http://example.com", "https://example.org"},
			expected:       "https://example.org",
		},
		{
			name:           "wildcard",
			origin:         "https://test.example.org",
			allowedOrigins: []string{"https://*.example.org"},
			expected:       "https://test.example.org",
		},
		{
			name:           "wildcard does not match a non-dot separator",
			origin:         "https://testxexample.org",
			allowedOrigins: []string{"https://*.example.org"},
		},
		{
			name:           "wildcard with different scheme",
			origin:         "http://test.example.org:443",
			allowedOrigins: []string{"https://*.example.org"},
		},
		{
			name:           "everything allowed plus specific domain",
			origin:         "https://example.org",
			allowedOrigins: []string{"*", "https://example.org"},
			expected:       "https://example.org",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			s := &httpp.Server{
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
			if ca.expected != "" {
				require.Equal(t, "Origin", res.Header.Get("Vary"))
			}
		})
	}
}

func TestHandlerOriginBlockPostRequests(t *testing.T) {
	for _, ca := range []struct {
		name           string
		fetchSite      string
		origin         string
		allowedOrigins []string
		expectedStatus int
		called         bool
	}{
		{
			name:           "without fetch metadata",
			expectedStatus: http.StatusOK,
			called:         true,
		},
		{
			name:           "same-origin without origin",
			fetchSite:      "same-origin",
			expectedStatus: http.StatusOK,
			called:         true,
		},
		{
			name:           "same-site with allowed origin",
			fetchSite:      "same-site",
			origin:         "https://example.com",
			allowedOrigins: []string{"https://example.com"},
			expectedStatus: http.StatusOK,
			called:         true,
		},
		{
			name:           "cross-site with allowed origin",
			fetchSite:      "cross-site",
			origin:         "https://example.com",
			allowedOrigins: []string{"https://example.com"},
			expectedStatus: http.StatusOK,
			called:         true,
		},
		{
			name:           "same-site without origin",
			fetchSite:      "same-site",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "cross-site without origin",
			fetchSite:      "cross-site",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "same-site with malformed origin",
			fetchSite:      "same-site",
			origin:         "://example.com",
			allowedOrigins: []string{"https://example.com"},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "cross-site with malformed origin",
			fetchSite:      "cross-site",
			origin:         "://example.com",
			allowedOrigins: []string{"https://example.com"},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "same-site with null origin",
			fetchSite:      "same-site",
			origin:         "null",
			allowedOrigins: []string{"https://example.com"},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "cross-site with null origin",
			fetchSite:      "cross-site",
			origin:         "null",
			allowedOrigins: []string{"https://example.com"},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "same-site with disallowed origin",
			fetchSite:      "same-site",
			origin:         "https://another.com",
			allowedOrigins: []string{"https://example.com"},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "cross-site with disallowed origin",
			fetchSite:      "cross-site",
			origin:         "https://another.com",
			allowedOrigins: []string{"https://example.com"},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "cross-site with wildcard origin",
			fetchSite:      "cross-site",
			origin:         "https://example.com",
			allowedOrigins: []string{"*"},
			expectedStatus: http.StatusOK,
			called:         true,
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			called := false

			s := &httpp.Server{
				Address:      "localhost:4555",
				AllowOrigins: ca.allowedOrigins,
				ReadTimeout:  10 * time.Second,
				WriteTimeout: 10 * time.Second,
				Parent:       test.NilLogger,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					called = true
					w.WriteHeader(http.StatusOK)
				}),
			}
			err := s.Initialize()
			require.NoError(t, err)
			defer s.Close()

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr}

			req, err := http.NewRequest(http.MethodPost, "http://localhost:4555", nil)
			require.NoError(t, err)

			req.Header.Set("Origin", ca.origin)
			req.Header.Set("Sec-Fetch-Site", ca.fetchSite)

			res, err := hc.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			require.Equal(t, ca.expectedStatus, res.StatusCode)
			require.Equal(t, ca.called, called)
		})
	}
}
