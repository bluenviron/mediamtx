package api //nolint:revive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/test"
)

type testParent struct {
	log  func(_ logger.Level, _ string, _ ...any)
	conf *conf.Conf
}

func (p *testParent) Log(l logger.Level, s string, a ...any) {
	if p.log != nil {
		p.log(l, s, a...)
	}
}

func (p *testParent) APIConfigSnapshot() *conf.Conf { return p.conf }
func (p *testParent) APIConfigGlobalPatch(in conf.OptionalGlobal) error {
	newConf := p.conf.Clone()
	newConf.PatchGlobal(&in)
	if err := newConf.Validate(nil); err != nil {
		return err
	}
	p.conf = newConf
	return nil
}

func (p *testParent) APIConfigPathDefaultsPatch(in conf.OptionalPath) error {
	newConf := p.conf.Clone()
	newConf.PatchPathDefaults(&in)
	if err := newConf.Validate(nil); err != nil {
		return err
	}
	p.conf = newConf
	return nil
}

func (p *testParent) APIConfigPathsAdd(name string, in conf.OptionalPath) error {
	newConf := p.conf.Clone()
	if err := newConf.AddPath(name, &in); err != nil {
		return err
	}
	if err := newConf.Validate(nil); err != nil {
		return err
	}
	p.conf = newConf
	return nil
}

func (p *testParent) APIConfigPathsPatch(name string, in conf.OptionalPath) error {
	newConf := p.conf.Clone()
	if err := newConf.PatchPath(name, &in); err != nil {
		return err
	}
	if err := newConf.Validate(nil); err != nil {
		return err
	}
	p.conf = newConf
	return nil
}

func (p *testParent) APIConfigPathsReplace(name string, in conf.OptionalPath) error {
	newConf := p.conf.Clone()
	if err := newConf.ReplacePath(name, &in); err != nil {
		return err
	}
	if err := newConf.Validate(nil); err != nil {
		return err
	}
	p.conf = newConf
	return nil
}

func (p *testParent) APIConfigPathsDelete(name string) error {
	newConf := p.conf.Clone()
	if err := newConf.RemovePath(name); err != nil {
		return err
	}
	if err := newConf.Validate(nil); err != nil {
		return err
	}
	p.conf = newConf
	return nil
}

func tempConf(t *testing.T, cnt string) *conf.Conf {
	fi := test.CreateTempFile(t, []byte(cnt))

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
		checkOK(t, res.Body)
		return
	}

	err = json.NewDecoder(res.Body).Decode(out)
	require.NoError(t, err)
}

func checkError(t *testing.T, body io.Reader, msg string) {
	var raw map[string]any
	err := json.NewDecoder(body).Decode(&raw)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"status": "error", "error": msg}, raw)
}

func checkOK(t *testing.T, body io.Reader) {
	var raw map[string]any
	err := json.NewDecoder(body).Decode(&raw)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"status": "ok"}, raw)
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

	req.Header.Add("Origin", "http://example.com")
	req.Header.Add("Access-Control-Request-Method", "GET")

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNoContent, res.StatusCode)

	byts, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	require.Equal(t, "*", res.Header.Get("Access-Control-Allow-Origin"))
	require.Equal(t, "OPTIONS, GET, POST, PATCH, DELETE", res.Header.Get("Access-Control-Allow-Methods"))
	require.Equal(t, "Authorization, Content-Type", res.Header.Get("Access-Control-Allow-Headers"))
	require.Equal(t, byts, []byte{})
}

func TestInfo(t *testing.T) {
	api := API{
		Version:      "v1.2.3",
		Started:      time.Date(2008, 11, 7, 11, 22, 0, 0, time.Local),
		Address:      "localhost:9997",
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
			AuthenticateImpl: func(_ *auth.Request) (string, *auth.Error) {
				return "", nil
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

	httpRequest(t, hc, http.MethodPost, u.String(), nil, nil)

	require.True(t, ok)
}

func TestAuthError(t *testing.T) {
	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager: &test.AuthManager{
			AuthenticateImpl: func(req *auth.Request) (string, *auth.Error) {
				if req.Credentials.User == "" {
					return "", &auth.Error{AskCredentials: true, Wrapped: fmt.Errorf("auth error")}
				}
				return "", &auth.Error{Wrapped: fmt.Errorf("auth error")}
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

	res, err := hc.Get("http://localhost:9997/v3/config/global/get")
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	require.Equal(t, `Basic realm="mediamtx"`, res.Header.Get("WWW-Authenticate"))
	checkError(t, res.Body, "authentication error")

	res, err = hc.Get("http://myuser:mypass@localhost:9997/v3/config/global/get")
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	require.Equal(t, ``, res.Header.Get("WWW-Authenticate"))
	checkError(t, res.Body, "authentication error")
}
