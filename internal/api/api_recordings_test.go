package api //nolint:revive

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestRecordingsList(t *testing.T) {
	dir := t.TempDir()

	cnf := tempConf(t, "pathDefaults:\n"+
		"  recordPath: "+filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f")+"\n"+
		"paths:\n"+
		"  mypath1:\n"+
		"  all_others:\n")

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
	dir := t.TempDir()

	cnf := tempConf(t, "pathDefaults:\n"+
		"  recordPath: "+filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f")+"\n"+
		"paths:\n"+
		"  all_others:\n")

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
	dir := t.TempDir()

	cnf := tempConf(t, "pathDefaults:\n"+
		"  recordPath: "+filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f")+"\n"+
		"paths:\n"+
		"  all_others:\n")

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

	err = os.MkdirAll(filepath.Join(dir, "group", "cam1"), 0o755)
	require.NoError(t, err)

	segmentPath := filepath.Join(dir, "group", "cam1", "2008-11-07_11-22-00-900000.mp4")
	err = os.WriteFile(segmentPath, []byte(""), 0o644)
	require.NoError(t, err)

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	u, err := url.Parse("http://localhost:9997/v3/recordings/segments/delete")
	require.NoError(t, err)

	v := url.Values{}
	v.Set("path", "group/cam1")
	v.Set("start", time.Date(2008, 11, 7, 11, 22, 0, 900000000, time.Local).Format(time.RFC3339Nano))
	u.RawQuery = v.Encode()

	req, err := http.NewRequest(http.MethodDelete, u.String(), nil)
	require.NoError(t, err)

	resp, err := hc.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	_, err = os.Stat(segmentPath)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

func TestRecordingsDeleteSegmentInvalidPath(t *testing.T) {
	dir := t.TempDir()

	cnf := tempConf(t, "pathDefaults:\n"+
		"  recordPath: "+filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f")+"\n"+
		"paths:\n"+
		"  all_others:\n")

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

	err = os.MkdirAll(filepath.Join(dir, "group", "cam1"), 0o755)
	require.NoError(t, err)

	segmentPath := filepath.Join(dir, "group", "cam1", "2008-11-07_11-22-00-900000.mp4")
	err = os.WriteFile(segmentPath, []byte(""), 0o644)
	require.NoError(t, err)
	collisionPath := filepath.Join(dir, "group", "cam1", "2008-11-07_11-22-00-900000~1.mp4")
	err = os.WriteFile(collisionPath, []byte(""), 0o644)
	require.NoError(t, err)

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	u, err := url.Parse("http://localhost:9997/v3/recordings/segments/delete")
	require.NoError(t, err)

	v := url.Values{}
	v.Set("path", "group/../cam1")
	v.Set("start", time.Date(2008, 11, 7, 11, 22, 0, 900000000, time.Local).Format(time.RFC3339Nano))
	u.RawQuery = v.Encode()

	req, err := http.NewRequest(http.MethodDelete, u.String(), nil)
	require.NoError(t, err)

	resp, err := hc.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_, err = os.Stat(segmentPath)
	require.NoError(t, err)
	require.FileExists(t, collisionPath)
}

func TestRecordingsDeleteSegmentCollisions(t *testing.T) {
	for _, ca := range []struct {
		name     string
		suffixes []string
	}{
		{"canonical_and_collisions", []string{"", "~1", "~2"}},
		{"suffix_only", []string{"~1"}},
	} {
		t.Run(ca.name, func(t *testing.T) {
			dir := t.TempDir()
			cnf := tempConf(t, "pathDefaults:\n"+
				"  recordPath: "+filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f")+"\n"+
				"paths:\n"+
				"  all_others:\n")
			api := API{
				Address:      "localhost:9997",
				ReadTimeout:  conf.Duration(10 * time.Second),
				WriteTimeout: conf.Duration(10 * time.Second),
				AuthManager:  test.NilAuthManager,
				Parent:       &testParent{conf: cnf},
			}
			require.NoError(t, api.Initialize())
			defer api.Close()

			segmentDir := filepath.Join(dir, "group", "cam1")
			require.NoError(t, os.MkdirAll(segmentDir, 0o755))
			segmentPaths := make([]string, 0, len(ca.suffixes))
			for _, suffix := range ca.suffixes {
				segmentPath := filepath.Join(segmentDir, "2008-11-07_11-22-00-900000"+suffix+".mp4")
				require.NoError(t, os.WriteFile(segmentPath, []byte("recording"), 0o644))
				segmentPaths = append(segmentPaths, segmentPath)
			}
			unrelatedPath := filepath.Join(segmentDir, "2008-11-07_11-22-01-900000.mp4")
			require.NoError(t, os.WriteFile(unrelatedPath, []byte("unrelated"), 0o644))

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr}
			values := url.Values{
				"path":  {"group/cam1"},
				"start": {time.Date(2008, 11, 7, 11, 22, 0, 900000000, time.Local).Format(time.RFC3339Nano)},
			}
			requestURL := "http://localhost:9997/v3/recordings/segments/delete?" + values.Encode()
			for i := 0; i <= len(segmentPaths); i++ {
				req, err := http.NewRequest(http.MethodDelete, requestURL, nil)
				require.NoError(t, err)
				resp, err := hc.Do(req)
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
				if i == len(segmentPaths) {
					require.Equal(t, http.StatusBadRequest, resp.StatusCode)
				} else {
					require.Equal(t, http.StatusOK, resp.StatusCode)
				}

				remaining := 0
				for _, segmentPath := range segmentPaths {
					_, err = os.Stat(segmentPath)
					if err == nil {
						remaining++
					} else {
						require.True(t, os.IsNotExist(err))
					}
				}
				require.Equal(t, max(len(segmentPaths)-i-1, 0), remaining)
				if ca.name == "canonical_and_collisions" {
					require.NoFileExists(t, segmentPaths[0])
				}
				b, err := os.ReadFile(unrelatedPath)
				require.NoError(t, err)
				require.Equal(t, []byte("unrelated"), b)
			}
		})
	}
}

func TestRecordingsSegmentGetInvalidPath(t *testing.T) {
	dir := t.TempDir()

	cnf := tempConf(t, "pathDefaults:\n"+
		"  recordPath: "+filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f")+"\n"+
		"paths:\n"+
		"  all_others:\n")

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

	resp, err := hc.Get("http://localhost:9997/v3/recordings/get/group/../cam1")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAbsolutePathInside(t *testing.T) {
	base := t.TempDir()

	inside, err := absolutePathInside(base, filepath.Join(base, "sub", "file.mp4"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(base, "sub", "file.mp4"), inside)

	_, err = absolutePathInside(base, filepath.Join(base, "..", "escape.mp4"))
	require.EqualError(t, err, "path escapes base directory")

	_, err = absolutePathInside(base, fmt.Sprintf("%s-sibling/../file.mp4", base))
	require.EqualError(t, err, "path escapes base directory")
}
