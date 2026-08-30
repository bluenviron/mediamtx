package api //nolint:revive

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestConfigPathDefaultsGet(t *testing.T) {
	cnf := tempConf(t, "api: yes\n"+
		"pathDefaults:\n"+
		"  readUser: myuser\n"+
		"  readPass: mypass\n")

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		Parent:       &testParent{conf: cnf},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var out map[string]any
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/pathdefaults/get", nil, &out)
	require.Equal(t, "publisher", out["source"])
	require.Equal(t, "<redacted>", out["readPass"])
}

func TestConfigPathDefaultsPatch(t *testing.T) {
	cnf := tempConf(t, "api: yes\n")

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		Parent:       &testParent{conf: cnf},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/pathdefaults/patch",
		map[string]any{
			"recordFormat": "fmp4",
		}, nil)

	time.Sleep(500 * time.Millisecond)

	var out map[string]any
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/pathdefaults/get", nil, &out)
	require.Equal(t, "fmp4", out["recordFormat"])
}
