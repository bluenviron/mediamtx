package api

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

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
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		Conf:         cnf,
		AuthManager:  test.NilAuthManager,
		Parent:       &testParent{},
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

	var out any
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/recordings/list", nil, &out)
	require.Equal(t, map[string]any{
		"itemCount": float64(2),
		"pageCount": float64(1),
		"items": []any{
			map[string]any{
				"name": "mypath1",
				"segments": []any{
					map[string]any{
						"start": time.Date(2008, 11, 7, 11, 22, 0, 500000000, time.Local).Format(time.RFC3339Nano),
					},
					map[string]any{
						"start": time.Date(2009, 11, 7, 11, 22, 0, 900000000, time.Local).Format(time.RFC3339Nano),
					},
				},
			},
			map[string]any{
				"name": "mypath2",
				"segments": []any{
					map[string]any{
						"start": time.Date(2009, 11, 7, 11, 22, 0, 900000000, time.Local).Format(time.RFC3339Nano),
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
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		Conf:         cnf,
		AuthManager:  test.NilAuthManager,
		Parent:       &testParent{},
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

	var out any
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/recordings/get/mypath1", nil, &out)
	require.Equal(t, map[string]any{
		"name": "mypath1",
		"segments": []any{
			map[string]any{
				"start": time.Date(2008, 11, 7, 11, 22, 0, 0, time.Local).Format(time.RFC3339Nano),
			},
			map[string]any{
				"start": time.Date(2009, 11, 7, 11, 22, 0, 900000000, time.Local).Format(time.RFC3339Nano),
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
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		Conf:         cnf,
		AuthManager:  test.NilAuthManager,
		Parent:       &testParent{},
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
	v.Set("start", time.Date(2008, 11, 7, 11, 22, 0, 900000000, time.Local).Format(time.RFC3339Nano))
	u.RawQuery = v.Encode()

	req, err := http.NewRequest(http.MethodDelete, u.String(), nil)
	require.NoError(t, err)

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
}
