package api //nolint:revive

import (
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestConfigPathDefaultsGet(t *testing.T) {
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

	var out map[string]any
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/pathdefaults/get", nil, &out)
	require.Equal(t, "publisher", out["source"])
}

func TestConfigPathDefaultsPatch(t *testing.T) {
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

	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/pathdefaults/patch",
		map[string]any{
			"recordFormat": "fmp4",
		}, nil)

	time.Sleep(500 * time.Millisecond)

	var out map[string]any
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/pathdefaults/get", nil, &out)
	require.Equal(t, "fmp4", out["recordFormat"])
}
