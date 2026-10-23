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
	"golang.org/x/sys/unix"

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

	test.WriteRecordingSegment(t, filepath.Join(dir, "mypath1", "2008-11-07_11-22-00-500000.mp4"),
		1500*time.Millisecond, true)

	test.WriteRecordingSegment(t, filepath.Join(dir, "mypath1", "2009-11-07_11-22-00-900000.mp4"),
		2250*time.Millisecond, false)

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
						"start":    time.Date(2008, 11, 7, 11, 22, 0, 500000000, time.Local).Format(time.RFC3339Nano),
						"duration": float64(1.5),
					},
					map[string]any{
						"start":    time.Date(2009, 11, 7, 11, 22, 0, 900000000, time.Local).Format(time.RFC3339Nano),
						"duration": float64(2.25),
					},
				},
			},
			map[string]any{
				"name": "mypath2",
				"segments": []any{
					map[string]any{
						"start":    time.Date(2009, 11, 7, 11, 22, 0, 900000000, time.Local).Format(time.RFC3339Nano),
						"duration": nil,
					},
				},
			},
		},
	}, out)
}

func TestRecordingsListPagination(t *testing.T) {
	dir := t.TempDir()

	cnf := tempConf(t, "pathDefaults:\n"+
		"  recordPath: "+filepath.Join(dir, "%path_%Y-%m-%d_%H-%M-%S-%f")+"\n"+
		"paths:\n"+
		"  all_others:\n")

	test.WriteRecordingSegment(t, filepath.Join(dir, "mypath1_2008-11-07_11-22-00-500000.mp4"),
		1500*time.Millisecond, true)
	secondPath := filepath.Join(dir, "mypath2_2008-11-07_11-22-00-500000.mp4")
	test.WriteRecordingSegment(t, secondPath, 2250*time.Millisecond, false)

	fd, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
	require.NoError(t, err)
	defer unix.Close(fd) //nolint:errcheck

	_, err = unix.InotifyAddWatch(fd, secondPath, unix.IN_OPEN)
	require.NoError(t, err)

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		Parent:       &testParent{conf: cnf},
	}
	err = api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	var out any
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/recordings/list?itemsPerPage=1", nil, &out)
	require.Equal(t, map[string]any{
		"itemCount": float64(2),
		"pageCount": float64(2),
		"items": []any{map[string]any{
			"name": "mypath1",
			"segments": []any{map[string]any{
				"start":    time.Date(2008, 11, 7, 11, 22, 0, 500000000, time.Local).Format(time.RFC3339Nano),
				"duration": float64(1.5),
			}},
		}},
	}, out)

	// segments outside the requested page must not be opened.
	buf := make([]byte, 4096)
	_, err = unix.Read(fd, buf)
	require.ErrorIs(t, err, unix.EAGAIN)

	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/recordings/list?itemsPerPage=1&page=1", nil, &out)
	require.Equal(t, map[string]any{
		"itemCount": float64(2),
		"pageCount": float64(2),
		"items": []any{map[string]any{
			"name": "mypath2",
			"segments": []any{map[string]any{
				"start":    time.Date(2008, 11, 7, 11, 22, 0, 500000000, time.Local).Format(time.RFC3339Nano),
				"duration": float64(2.25),
			}},
		}},
	}, out)

	n, err := unix.Read(fd, buf)
	require.NoError(t, err)
	require.Positive(t, n)

	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/recordings/list?itemsPerPage=1&page=2", nil, &out)
	require.Equal(t, map[string]any{
		"itemCount": float64(2),
		"pageCount": float64(2),
		"items":     []any{},
	}, out)

	_, err = unix.Read(fd, buf)
	require.ErrorIs(t, err, unix.EAGAIN)
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

	test.WriteRecordingSegment(t, filepath.Join(dir, "mypath1", "2008-11-07_11-22-00-000000.mp4"),
		1500*time.Millisecond, true)

	test.WriteRecordingSegment(t, filepath.Join(dir, "mypath1", "2009-11-07_11-22-00-900000.mp4"),
		2250*time.Millisecond, false)

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var out any
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/recordings/get/mypath1", nil, &out)
	require.Equal(t, map[string]any{
		"name": "mypath1",
		"segments": []any{
			map[string]any{
				"start":    time.Date(2008, 11, 7, 11, 22, 0, 0, time.Local).Format(time.RFC3339Nano),
				"duration": float64(1.5),
			},
			map[string]any{
				"start":    time.Date(2009, 11, 7, 11, 22, 0, 900000000, time.Local).Format(time.RFC3339Nano),
				"duration": float64(2.25),
			},
		},
	}, out)
}

func TestRecordingsDurationUnavailable(t *testing.T) {
	for _, ca := range []string{"fmp4", "mpegts"} {
		t.Run(ca, func(t *testing.T) {
			dir := t.TempDir()

			cnf := tempConf(t, "pathDefaults:\n"+
				"  recordPath: "+filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f")+"\n"+
				"  recordFormat: "+ca+"\n"+
				"paths:\n"+
				"  all_others:\n")

			err := os.Mkdir(filepath.Join(dir, "mypath"), 0o755)
			require.NoError(t, err)

			extension := ".mp4"
			if ca == "mpegts" {
				extension = ".ts"
			}
			err = os.WriteFile(filepath.Join(dir, "mypath", "2008-11-07_11-22-00-500000"+extension), []byte{}, 0o644)
			require.NoError(t, err)

			api := API{
				Address:      "localhost:9997",
				ReadTimeout:  conf.Duration(10 * time.Second),
				WriteTimeout: conf.Duration(10 * time.Second),
				AuthManager:  test.NilAuthManager,
				Parent:       &testParent{conf: cnf},
			}
			err = api.Initialize()
			require.NoError(t, err)
			defer api.Close()

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr, Timeout: 10 * time.Second}

			recording := map[string]any{
				"name": "mypath",
				"segments": []any{map[string]any{
					"start":    time.Date(2008, 11, 7, 11, 22, 0, 500000000, time.Local).Format(time.RFC3339Nano),
					"duration": nil,
				}},
			}

			var out any
			httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/recordings/get/mypath", nil, &out)
			require.Equal(t, recording, out)

			httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/recordings/list", nil, &out)
			require.Equal(t, map[string]any{
				"itemCount": float64(1),
				"pageCount": float64(1),
				"items":     []any{recording},
			}, out)

			httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/recordings/get/empty", nil, &out)
			require.Equal(t, map[string]any{
				"name":     "empty",
				"segments": []any{},
			}, out)
		})
	}
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
