package playback

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestPreflightRequest(t *testing.T) {
	s := &Server{
		Address:      "127.0.0.1:9996",
		AllowOrigin:  "*",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		Parent:       test.NilLogger,
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	req, err := http.NewRequest(http.MethodOptions, "http://localhost:9996", nil)
	require.NoError(t, err)

	req.Header.Add("Access-Control-Request-Method", "GET")

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNoContent, res.StatusCode)

	byts, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	require.Equal(t, "*", res.Header.Get("Access-Control-Allow-Origin"))
	require.Equal(t, "true", res.Header.Get("Access-Control-Allow-Credentials"))
	require.Equal(t, "OPTIONS, GET", res.Header.Get("Access-Control-Allow-Methods"))
	require.Equal(t, "Authorization", res.Header.Get("Access-Control-Allow-Headers"))
	require.Equal(t, byts, []byte{})
}

func TestAuthError(t *testing.T) {
	n := 0

	s := &Server{
		Address:      "127.0.0.1:9996",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager: &test.AuthManager{
			AuthenticateImpl: func(req *auth.Request) *auth.Error {
				if req.Credentials.User == "" {
					return &auth.Error{AskCredentials: true}
				}
				return &auth.Error{Wrapped: fmt.Errorf("auth error")}
			},
		},
		Parent: test.Logger(func(l logger.Level, s string, i ...any) {
			if l == logger.Info {
				if n == 1 {
					require.Regexp(t, "failed to authenticate: auth error$", fmt.Sprintf(s, i...))
				}
				n++
			}
		}),
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	u, err := url.Parse("http://localhost:9996/list")
	require.NoError(t, err)

	v := url.Values{}
	v.Set("path", "mypath")
	u.RawQuery = v.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	require.Equal(t, `Basic realm="mediamtx"`, res.Header.Get("WWW-Authenticate"))

	u, err = url.Parse("http://myuser:mypass@localhost:9996/list")
	require.NoError(t, err)

	v = url.Values{}
	v.Set("path", "mypath")
	u.RawQuery = v.Encode()

	req, err = http.NewRequest(http.MethodGet, u.String(), nil)
	require.NoError(t, err)

	start := time.Now()

	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Greater(t, time.Since(start), 2*time.Second)

	require.Equal(t, http.StatusUnauthorized, res.StatusCode)

	require.Equal(t, 2, n)
}
