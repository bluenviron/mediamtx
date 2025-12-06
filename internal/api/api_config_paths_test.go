package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestConfigPathsList(t *testing.T) {
	cnf := tempConf(t, "api: yes\n"+
		"paths:\n"+
		"  path1:\n"+
		"    readUser: myuser1\n"+
		"    readPass: mypass1\n"+
		"  path2:\n"+
		"    readUser: myuser2\n"+
		"    readPass: mypass2\n")

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

	type pathConfig map[string]any

	type listRes struct {
		ItemCount int          `json:"itemCount"`
		PageCount int          `json:"pageCount"`
		Items     []pathConfig `json:"items"`
	}

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var out listRes
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/paths/list", nil, &out)
	require.Equal(t, 2, out.ItemCount)
	require.Equal(t, 1, out.PageCount)
	require.Equal(t, "path1", out.Items[0]["name"])
	require.Equal(t, "myuser1", out.Items[0]["readUser"])
	require.Equal(t, "mypass1", out.Items[0]["readPass"])
	require.Equal(t, "path2", out.Items[1]["name"])
	require.Equal(t, "myuser2", out.Items[1]["readUser"])
	require.Equal(t, "mypass2", out.Items[1]["readPass"])
}

func TestConfigPathsGet(t *testing.T) {
	cnf := tempConf(t, "api: yes\n"+
		"paths:\n"+
		"  my/path:\n"+
		"    readUser: myuser\n"+
		"    readPass: mypass\n")

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
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/paths/get/my/path", nil, &out)
	require.Equal(t, "my/path", out["name"])
	require.Equal(t, "myuser", out["readUser"])
}

func TestConfigPathsAdd(t *testing.T) {
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

	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/config/paths/add/my/path",
		map[string]any{
			"source":                   "rtsp://127.0.0.1:9999/mypath",
			"sourceOnDemand":           true,
			"disablePublisherOverride": true, // test setting a deprecated parameter
			"rpiCameraVFlip":           true,
		}, nil)

	var out map[string]any
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/paths/get/my/path", nil, &out)
	require.Equal(t, "rtsp://127.0.0.1:9999/mypath", out["source"])
	require.Equal(t, true, out["sourceOnDemand"])
	require.Equal(t, true, out["disablePublisherOverride"])
	require.Equal(t, true, out["rpiCameraVFlip"])
}

func TestConfigPathsAddUnknownField(t *testing.T) { //nolint:dupl
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

	req, err := http.NewRequest(http.MethodPost,
		"http://localhost:9997/v3/config/paths/add/my/path", bytes.NewReader(byts))
	require.NoError(t, err)

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusBadRequest, res.StatusCode)
	checkError(t, res.Body, "json: unknown field \"test\"")
}

func TestConfigPathsPatch(t *testing.T) { //nolint:dupl
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

	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/config/paths/add/my/path",
		map[string]any{
			"source":                   "rtsp://127.0.0.1:9999/mypath",
			"sourceOnDemand":           true,
			"disablePublisherOverride": true, // test setting a deprecated parameter
			"rpiCameraVFlip":           true,
		}, nil)

	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/paths/patch/my/path",
		map[string]any{
			"source":         "rtsp://127.0.0.1:9998/mypath",
			"sourceOnDemand": true,
		}, nil)

	var out map[string]any
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/paths/get/my/path", nil, &out)
	require.Equal(t, "rtsp://127.0.0.1:9998/mypath", out["source"])
	require.Equal(t, true, out["sourceOnDemand"])
	require.Equal(t, true, out["disablePublisherOverride"])
	require.Equal(t, true, out["rpiCameraVFlip"])
}

func TestConfigPathsReplace(t *testing.T) { //nolint:dupl
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

	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/config/paths/add/my/path",
		map[string]any{
			"source":                   "rtsp://127.0.0.1:9999/mypath",
			"sourceOnDemand":           true,
			"disablePublisherOverride": true, // test setting a deprecated parameter
			"rpiCameraVFlip":           true,
		}, nil)

	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/config/paths/replace/my/path",
		map[string]any{
			"source":         "rtsp://127.0.0.1:9998/mypath",
			"sourceOnDemand": true,
		}, nil)

	var out map[string]any
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/paths/get/my/path", nil, &out)
	require.Equal(t, "rtsp://127.0.0.1:9998/mypath", out["source"])
	require.Equal(t, true, out["sourceOnDemand"])
	require.Equal(t, nil, out["disablePublisherOverride"])
	require.Equal(t, false, out["rpiCameraVFlip"])
}

func TestConfigPathsReplaceNonExisting(t *testing.T) { //nolint:dupl
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

	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/config/paths/replace/my/path",
		map[string]any{
			"source":         "rtsp://127.0.0.1:9998/mypath",
			"sourceOnDemand": true,
		}, nil)

	var out map[string]any
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/paths/get/my/path", nil, &out)
	require.Equal(t, "rtsp://127.0.0.1:9998/mypath", out["source"])
	require.Equal(t, true, out["sourceOnDemand"])
	require.Equal(t, nil, out["disablePublisherOverride"])
	require.Equal(t, false, out["rpiCameraVFlip"])
}

func TestConfigPathsDelete(t *testing.T) {
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

	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/config/paths/add/my/path",
		map[string]any{
			"source":         "rtsp://127.0.0.1:9999/mypath",
			"sourceOnDemand": true,
		}, nil)

	httpRequest(t, hc, http.MethodDelete, "http://localhost:9997/v3/config/paths/delete/my/path", nil, nil)

	req, err := http.NewRequest(http.MethodGet, "http://localhost:9997/v3/config/paths/get/my/path", nil)
	require.NoError(t, err)

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNotFound, res.StatusCode)
	checkError(t, res.Body, "path configuration not found")
}
