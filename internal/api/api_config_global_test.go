package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestConfigGlobalGet(t *testing.T) {
	cnf := tempConf(t, "api: yes\n")
	checked := false

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		Conf:         cnf,
		AuthManager: &test.AuthManager{
			AuthenticateImpl: func(req *auth.Request) *auth.Error {
				require.Equal(t, conf.AuthActionAPI, req.Action)
				require.Equal(t, "myuser", req.Credentials.User)
				require.Equal(t, "mypass", req.Credentials.Pass)
				checked = true
				return nil
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

	var out map[string]any
	httpRequest(t, hc, http.MethodGet, "http://myuser:mypass@localhost:9997/v3/config/global/get", nil, &out)
	require.Equal(t, true, out["api"])

	require.True(t, checked)
}

func TestConfigGlobalPatch(t *testing.T) {
	cnf := tempConf(t, "api: yes\n")

	api := API{
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

	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/global/patch",
		map[string]any{
			"rtmp":            false,
			"readTimeout":     "7s",
			"protocols":       []string{"tcp"},
			"readBufferCount": 4096, // test setting a deprecated parameter
		}, nil)

	time.Sleep(500 * time.Millisecond)

	var out map[string]any
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/global/get", nil, &out)
	require.Equal(t, false, out["rtmp"])
	require.Equal(t, "7s", out["readTimeout"])
	require.Equal(t, []any{"tcp"}, out["protocols"])
	require.Equal(t, float64(4096), out["readBufferCount"])
}

func TestConfigGlobalPatchUnknownField(t *testing.T) { //nolint:dupl
	cnf := tempConf(t, "api: yes\n")

	api := API{
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

	b := map[string]any{
		"test": "asd",
	}

	byts, err := json.Marshal(b)
	require.NoError(t, err)

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	req, err := http.NewRequest(http.MethodPatch, "http://localhost:9997/v3/config/global/patch",
		bytes.NewReader(byts))
	require.NoError(t, err)

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusBadRequest, res.StatusCode)
	checkError(t, res.Body, "json: unknown field \"test\"")
}
