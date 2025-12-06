package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

type testParent struct {
	log func(_ logger.Level, _ string, _ ...any)
}

func (p testParent) Log(l logger.Level, s string, a ...any) {
	if p.log != nil {
		p.log(l, s, a...)
	}
}

func (testParent) APIConfigSet(_ *conf.Conf) {}

func tempConf(t *testing.T, cnt string) *conf.Conf {
	fi, err := test.CreateTempFile([]byte(cnt))
	require.NoError(t, err)
	defer os.Remove(fi)

	cnf, _, err := conf.Load(fi, nil, nil)
	require.NoError(t, err)

	return cnf
}

func httpRequest(t *testing.T, hc *http.Client, method string, ur string, in any, out any) {
	buf := func() io.Reader {
		if in == nil {
			return nil
		}

		byts, err := json.Marshal(in)
		require.NoError(t, err)

		return bytes.NewBuffer(byts)
	}()

	req, err := http.NewRequest(method, ur, buf)
	require.NoError(t, err)

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("bad status code: %d", res.StatusCode)
	}

	if out == nil {
		return
	}

	err = json.NewDecoder(res.Body).Decode(out)
	require.NoError(t, err)
}

func checkError(t *testing.T, body io.Reader, msg string) {
	var resErr map[string]any
	err := json.NewDecoder(body).Decode(&resErr)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"error": msg}, resErr)
}

func TestPreflightRequest(t *testing.T) {
	api := API{
		Address:      "localhost:9997",
		AllowOrigins: []string{"*"},
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		Parent:       &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	req, err := http.NewRequest(http.MethodOptions, "http://localhost:9997", nil)
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
	require.Equal(t, "OPTIONS, GET, POST, PATCH, DELETE", res.Header.Get("Access-Control-Allow-Methods"))
	require.Equal(t, "Authorization, Content-Type", res.Header.Get("Access-Control-Allow-Headers"))
	require.Equal(t, byts, []byte{})
}

func TestInfo(t *testing.T) {
	cnf := tempConf(t, "api: yes\n")

	api := API{
		Version:      "v1.2.3",
		Started:      time.Date(2008, 11, 7, 11, 22, 0, 0, time.Local),
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		Conf:         cnf,
		AuthManager:  test.NilAuthManager,
		Parent:       &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var out map[string]any
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/info", nil, &out)
	require.Equal(t, map[string]any{
		"started": time.Date(2008, 11, 7, 11, 22, 0, 0, time.Local).Format(time.RFC3339),
		"version": "v1.2.3",
	}, out)
}

func TestAuthJWKSRefresh(t *testing.T) {
	ok := false

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager: &test.AuthManager{
			AuthenticateImpl: func(_ *auth.Request) *auth.Error {
				return nil
			},
			RefreshJWTJWKSImpl: func() {
				ok = true
			},
		},
		Parent: &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	u, err := url.Parse("http://localhost:9997/v3/auth/jwks/refresh")
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, u.String(), nil)
	require.NoError(t, err)

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	require.True(t, ok)
}

func TestAuthError(t *testing.T) {
	cnf := tempConf(t, "api: yes\n")
	n := 0

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		Conf:         cnf,
		AuthManager: &test.AuthManager{
			AuthenticateImpl: func(req *auth.Request) *auth.Error {
				if req.Credentials.User == "" {
					return &auth.Error{AskCredentials: true}
				}
				return &auth.Error{Wrapped: fmt.Errorf("auth error")}
			},
		},
		Parent: &testParent{
			log: func(l logger.Level, s string, i ...any) {
				if l == logger.Info {
					if n == 1 {
						require.Regexp(t, "failed to authenticate: auth error$", fmt.Sprintf(s, i...))
					}
					n++
				}
			},
		},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	res, err := hc.Get("http://localhost:9997/v3/config/global/get")
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	require.Equal(t, `Basic realm="mediamtx"`, res.Header.Get("WWW-Authenticate"))

	res, err = hc.Get("http://myuser:mypass@localhost:9997/v3/config/global/get")
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusUnauthorized, res.StatusCode)

	require.Equal(t, 2, n)
}
