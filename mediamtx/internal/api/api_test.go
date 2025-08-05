package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

type testParent struct{}

func (testParent) Log(_ logger.Level, _ string, _ ...interface{}) {
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

func httpRequest(t *testing.T, hc *http.Client, method string, ur string, in interface{}, out interface{}) {
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

func checkError(t *testing.T, msg string, body io.Reader) {
	var resErr map[string]interface{}
	err := json.NewDecoder(body).Decode(&resErr)
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{"error": msg}, resErr)
}

func TestPreflightRequest(t *testing.T) {
	api := API{
		Address:     "localhost:9997",
		AllowOrigin: "*",
		ReadTimeout: conf.Duration(10 * time.Second),
		AuthManager: test.NilAuthManager,
		Parent:      &testParent{},
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

func TestConfigGlobalGet(t *testing.T) {
	cnf := tempConf(t, "api: yes\n")

	api := API{
		Address:     "localhost:9997",
		ReadTimeout: conf.Duration(10 * time.Second),
		Conf:        cnf,
		AuthManager: test.NilAuthManager,
		Parent:      &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var out map[string]interface{}
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/global/get", nil, &out)
	require.Equal(t, true, out["api"])
}

func TestConfigGlobalPatch(t *testing.T) {
	cnf := tempConf(t, "api: yes\n")

	api := API{
		Address:     "localhost:9997",
		ReadTimeout: conf.Duration(10 * time.Second),
		Conf:        cnf,
		AuthManager: test.NilAuthManager,
		Parent:      &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/global/patch",
		map[string]interface{}{
			"rtmp":            false,
			"readTimeout":     "7s",
			"protocols":       []string{"tcp"},
			"readBufferCount": 4096, // test setting a deprecated parameter
		}, nil)

	time.Sleep(500 * time.Millisecond)

	var out map[string]interface{}
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/global/get", nil, &out)
	require.Equal(t, false, out["rtmp"])
	require.Equal(t, "7s", out["readTimeout"])
	require.Equal(t, []interface{}{"tcp"}, out["protocols"])
	require.Equal(t, float64(4096), out["readBufferCount"])
}

func TestConfigGlobalPatchUnknownField(t *testing.T) { //nolint:dupl
	cnf := tempConf(t, "api: yes\n")

	api := API{
		Address:     "localhost:9997",
		ReadTimeout: conf.Duration(10 * time.Second),
		Conf:        cnf,
		AuthManager: test.NilAuthManager,
		Parent:      &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	b := map[string]interface{}{
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
	checkError(t, "json: unknown field \"test\"", res.Body)
}

func TestConfigPathDefaultsGet(t *testing.T) {
	cnf := tempConf(t, "api: yes\n")

	api := API{
		Address:     "localhost:9997",
		ReadTimeout: conf.Duration(10 * time.Second),
		Conf:        cnf,
		AuthManager: test.NilAuthManager,
		Parent:      &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var out map[string]interface{}
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/pathdefaults/get", nil, &out)
	require.Equal(t, "publisher", out["source"])
}

func TestConfigPathDefaultsPatch(t *testing.T) {
	cnf := tempConf(t, "api: yes\n")

	api := API{
		Address:     "localhost:9997",
		ReadTimeout: conf.Duration(10 * time.Second),
		Conf:        cnf,
		AuthManager: test.NilAuthManager,
		Parent:      &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/pathdefaults/patch",
		map[string]interface{}{
			"recordFormat": "fmp4",
		}, nil)

	time.Sleep(500 * time.Millisecond)

	var out map[string]interface{}
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/pathdefaults/get", nil, &out)
	require.Equal(t, "fmp4", out["recordFormat"])
}

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
		Address:     "localhost:9997",
		ReadTimeout: conf.Duration(10 * time.Second),
		Conf:        cnf,
		AuthManager: test.NilAuthManager,
		Parent:      &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	type pathConfig map[string]interface{}

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
		Address:     "localhost:9997",
		ReadTimeout: conf.Duration(10 * time.Second),
		Conf:        cnf,
		AuthManager: test.NilAuthManager,
		Parent:      &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var out map[string]interface{}
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/paths/get/my/path", nil, &out)
	require.Equal(t, "my/path", out["name"])
	require.Equal(t, "myuser", out["readUser"])
}

func TestConfigPathsAdd(t *testing.T) {
	cnf := tempConf(t, "api: yes\n")

	api := API{
		Address:     "localhost:9997",
		ReadTimeout: conf.Duration(10 * time.Second),
		Conf:        cnf,
		AuthManager: test.NilAuthManager,
		Parent:      &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/config/paths/add/my/path",
		map[string]interface{}{
			"source":                   "rtsp://127.0.0.1:9999/mypath",
			"sourceOnDemand":           true,
			"disablePublisherOverride": true, // test setting a deprecated parameter
			"rpiCameraVFlip":           true,
		}, nil)

	var out map[string]interface{}
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/paths/get/my/path", nil, &out)
	require.Equal(t, "rtsp://127.0.0.1:9999/mypath", out["source"])
	require.Equal(t, true, out["sourceOnDemand"])
	require.Equal(t, true, out["disablePublisherOverride"])
	require.Equal(t, true, out["rpiCameraVFlip"])
}

func TestConfigPathsAddUnknownField(t *testing.T) { //nolint:dupl
	cnf := tempConf(t, "api: yes\n")

	api := API{
		Address:     "localhost:9997",
		ReadTimeout: conf.Duration(10 * time.Second),
		Conf:        cnf,
		AuthManager: test.NilAuthManager,
		Parent:      &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	b := map[string]interface{}{
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
	checkError(t, "json: unknown field \"test\"", res.Body)
}

func TestConfigPathsPatch(t *testing.T) { //nolint:dupl
	cnf := tempConf(t, "api: yes\n")

	api := API{
		Address:     "localhost:9997",
		ReadTimeout: conf.Duration(10 * time.Second),
		Conf:        cnf,
		AuthManager: test.NilAuthManager,
		Parent:      &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/config/paths/add/my/path",
		map[string]interface{}{
			"source":                   "rtsp://127.0.0.1:9999/mypath",
			"sourceOnDemand":           true,
			"disablePublisherOverride": true, // test setting a deprecated parameter
			"rpiCameraVFlip":           true,
		}, nil)

	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/paths/patch/my/path",
		map[string]interface{}{
			"source":         "rtsp://127.0.0.1:9998/mypath",
			"sourceOnDemand": true,
		}, nil)

	var out map[string]interface{}
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/paths/get/my/path", nil, &out)
	require.Equal(t, "rtsp://127.0.0.1:9998/mypath", out["source"])
	require.Equal(t, true, out["sourceOnDemand"])
	require.Equal(t, true, out["disablePublisherOverride"])
	require.Equal(t, true, out["rpiCameraVFlip"])
}

func TestConfigPathsReplace(t *testing.T) { //nolint:dupl
	cnf := tempConf(t, "api: yes\n")

	api := API{
		Address:     "localhost:9997",
		ReadTimeout: conf.Duration(10 * time.Second),
		Conf:        cnf,
		AuthManager: test.NilAuthManager,
		Parent:      &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/config/paths/add/my/path",
		map[string]interface{}{
			"source":                   "rtsp://127.0.0.1:9999/mypath",
			"sourceOnDemand":           true,
			"disablePublisherOverride": true, // test setting a deprecated parameter
			"rpiCameraVFlip":           true,
		}, nil)

	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/config/paths/replace/my/path",
		map[string]interface{}{
			"source":         "rtsp://127.0.0.1:9998/mypath",
			"sourceOnDemand": true,
		}, nil)

	var out map[string]interface{}
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/paths/get/my/path", nil, &out)
	require.Equal(t, "rtsp://127.0.0.1:9998/mypath", out["source"])
	require.Equal(t, true, out["sourceOnDemand"])
	require.Equal(t, nil, out["disablePublisherOverride"])
	require.Equal(t, false, out["rpiCameraVFlip"])
}

func TestConfigPathsReplaceNonExisting(t *testing.T) { //nolint:dupl
	cnf := tempConf(t, "api: yes\n")

	api := API{
		Address:     "localhost:9997",
		ReadTimeout: conf.Duration(10 * time.Second),
		Conf:        cnf,
		AuthManager: test.NilAuthManager,
		Parent:      &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/config/paths/replace/my/path",
		map[string]interface{}{
			"source":         "rtsp://127.0.0.1:9998/mypath",
			"sourceOnDemand": true,
		}, nil)

	var out map[string]interface{}
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/paths/get/my/path", nil, &out)
	require.Equal(t, "rtsp://127.0.0.1:9998/mypath", out["source"])
	require.Equal(t, true, out["sourceOnDemand"])
	require.Equal(t, nil, out["disablePublisherOverride"])
	require.Equal(t, false, out["rpiCameraVFlip"])
}

func TestConfigPathsDelete(t *testing.T) {
	cnf := tempConf(t, "api: yes\n")

	api := API{
		Address:     "localhost:9997",
		ReadTimeout: conf.Duration(10 * time.Second),
		Conf:        cnf,
		AuthManager: test.NilAuthManager,
		Parent:      &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/config/paths/add/my/path",
		map[string]interface{}{
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
	checkError(t, "path configuration not found", res.Body)
}

func TestRecordingsList(t *testing.T) {
	dir, err := os.MkdirTemp("", "mediamtx-playback")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	cnf := tempConf(t, "pathDefaults:\n"+
		"  recordPath: "+filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f")+"\n"+
		"paths:\n"+
		"  mypath1:\n"+
		"  all_others:\n")

	api := API{
		Address:     "localhost:9997",
		ReadTimeout: conf.Duration(10 * time.Second),
		Conf:        cnf,
		AuthManager: test.NilAuthManager,
		Parent:      &testParent{},
	}
	err = api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	err = os.Mkdir(filepath.Join(dir, "mypath1"), 0o755)
	require.NoError(t, err)

	err = os.Mkdir(filepath.Join(dir, "mypath2"), 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "mypath1", "2008-11-07_11-22-00-500000.mp4"), []byte(""), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "mypath1", "2009-11-07_11-22-00-900000.mp4"), []byte(""), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "mypath2", "2009-11-07_11-22-00-900000.mp4"), []byte(""), 0o644)
	require.NoError(t, err)

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var out interface{}
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/recordings/list", nil, &out)
	require.Equal(t, map[string]interface{}{
		"itemCount": float64(2),
		"pageCount": float64(1),
		"items": []interface{}{
			map[string]interface{}{
				"name": "mypath1",
				"segments": []interface{}{
					map[string]interface{}{
						"start": time.Date(2008, 11, 0o7, 11, 22, 0, 500000000, time.Local).Format(time.RFC3339Nano),
					},
					map[string]interface{}{
						"start": time.Date(2009, 11, 0o7, 11, 22, 0, 900000000, time.Local).Format(time.RFC3339Nano),
					},
				},
			},
			map[string]interface{}{
				"name": "mypath2",
				"segments": []interface{}{
					map[string]interface{}{
						"start": time.Date(2009, 11, 0o7, 11, 22, 0, 900000000, time.Local).Format(time.RFC3339Nano),
					},
				},
			},
		},
	}, out)
}

func TestRecordingsGet(t *testing.T) {
	dir, err := os.MkdirTemp("", "mediamtx-playback")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	cnf := tempConf(t, "pathDefaults:\n"+
		"  recordPath: "+filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f")+"\n"+
		"paths:\n"+
		"  all_others:\n")

	api := API{
		Address:     "localhost:9997",
		ReadTimeout: conf.Duration(10 * time.Second),
		Conf:        cnf,
		AuthManager: test.NilAuthManager,
		Parent:      &testParent{},
	}
	err = api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	err = os.Mkdir(filepath.Join(dir, "mypath1"), 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "mypath1", "2008-11-07_11-22-00-000000.mp4"), []byte(""), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "mypath1", "2009-11-07_11-22-00-900000.mp4"), []byte(""), 0o644)
	require.NoError(t, err)

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var out interface{}
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/recordings/get/mypath1", nil, &out)
	require.Equal(t, map[string]interface{}{
		"name": "mypath1",
		"segments": []interface{}{
			map[string]interface{}{
				"start": time.Date(2008, 11, 0o7, 11, 22, 0, 0, time.Local).Format(time.RFC3339Nano),
			},
			map[string]interface{}{
				"start": time.Date(2009, 11, 0o7, 11, 22, 0, 900000000, time.Local).Format(time.RFC3339Nano),
			},
		},
	}, out)
}

func TestRecordingsDeleteSegment(t *testing.T) {
	dir, err := os.MkdirTemp("", "mediamtx-playback")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	cnf := tempConf(t, "pathDefaults:\n"+
		"  recordPath: "+filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f")+"\n"+
		"paths:\n"+
		"  all_others:\n")

	api := API{
		Address:     "localhost:9997",
		ReadTimeout: conf.Duration(10 * time.Second),
		Conf:        cnf,
		AuthManager: test.NilAuthManager,
		Parent:      &testParent{},
	}
	err = api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	err = os.Mkdir(filepath.Join(dir, "mypath1"), 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "mypath1", "2008-11-07_11-22-00-900000.mp4"), []byte(""), 0o644)
	require.NoError(t, err)

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	u, err := url.Parse("http://localhost:9997/v3/recordings/deletesegment")
	require.NoError(t, err)

	v := url.Values{}
	v.Set("path", "mypath1")
	v.Set("start", time.Date(2008, 11, 0o7, 11, 22, 0, 900000000, time.Local).Format(time.RFC3339Nano))
	u.RawQuery = v.Encode()

	req, err := http.NewRequest(http.MethodDelete, u.String(), nil)
	require.NoError(t, err)

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
}

func TestAuthJWKSRefresh(t *testing.T) {
	ok := false

	api := API{
		Address:     "localhost:9997",
		ReadTimeout: conf.Duration(10 * time.Second),
		AuthManager: &test.AuthManager{
			AuthenticateImpl: func(_ *auth.Request) error {
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
