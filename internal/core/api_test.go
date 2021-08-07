package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/stretchr/testify/require"
)

func httpRequest(method string, ur string, in interface{}, out interface{}) error {
	buf, err := func() (io.Reader, error) {
		if in == nil {
			return nil, nil
		}

		byts, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}

		return bytes.NewBuffer(byts), nil
	}()
	if err != nil {
		return err
	}

	req, err := http.NewRequest(method, ur, buf)
	if err != nil {
		return err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status code: %d", res.StatusCode)
	}

	if out == nil {
		return nil
	}

	return json.NewDecoder(res.Body).Decode(out)
}

func TestAPIConfigGet(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.close()

	var out map[string]interface{}
	err := httpRequest(http.MethodGet, "http://localhost:9997/config/get", nil, &out)
	require.NoError(t, err)
	require.Equal(t, true, out["api"])
}

func TestAPIConfigSet(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.close()

	err := httpRequest(http.MethodPost, "http://localhost:9997/config/set", map[string]interface{}{
		"rtmpDisable": true,
	}, nil)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	var out map[string]interface{}
	err = httpRequest(http.MethodGet, "http://localhost:9997/config/get", nil, &out)
	require.NoError(t, err)
	require.Equal(t, true, out["rtmpDisable"])
}

func TestAPIConfigPathsAdd(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.close()

	err := httpRequest(http.MethodPost, "http://localhost:9997/config/paths/add/mypath", map[string]interface{}{
		"source":         "rtsp://127.0.0.1:9999/mypath",
		"sourceOnDemand": true,
	}, nil)
	require.NoError(t, err)

	var out map[string]interface{}
	err = httpRequest(http.MethodGet, "http://localhost:9997/config/get", nil, &out)
	require.NoError(t, err)
	require.Equal(t, "rtsp://127.0.0.1:9999/mypath", out["paths"].(map[string]interface{})["mypath"].(map[string]interface{})["source"])
}

func TestAPIConfigPathsEdit(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.close()

	err := httpRequest(http.MethodPost, "http://localhost:9997/config/paths/add/mypath", map[string]interface{}{
		"source":         "rtsp://127.0.0.1:9999/mypath",
		"sourceOnDemand": true,
	}, nil)
	require.NoError(t, err)

	err = httpRequest(http.MethodPost, "http://localhost:9997/config/paths/edit/mypath", map[string]interface{}{
		"source":         "rtsp://127.0.0.1:9998/mypath",
		"sourceOnDemand": true,
	}, nil)
	require.NoError(t, err)

	var out struct {
		Paths map[string]struct {
			Source string `json:"source"`
		} `json:"paths"`
	}
	err = httpRequest(http.MethodGet, "http://localhost:9997/config/get", nil, &out)
	require.NoError(t, err)
	require.Equal(t, "rtsp://127.0.0.1:9998/mypath", out.Paths["mypath"].Source)
}

func TestAPIConfigPathsRemove(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.close()

	err := httpRequest(http.MethodPost, "http://localhost:9997/config/paths/add/mypath", map[string]interface{}{
		"source":         "rtsp://127.0.0.1:9999/mypath",
		"sourceOnDemand": true,
	}, nil)
	require.NoError(t, err)

	err = httpRequest(http.MethodPost, "http://localhost:9997/config/paths/remove/mypath", nil, nil)
	require.NoError(t, err)

	var out struct {
		Paths map[string]interface{} `json:"paths"`
	}
	err = httpRequest(http.MethodGet, "http://localhost:9997/config/get", nil, &out)
	require.NoError(t, err)
	_, ok = out.Paths["mypath"]
	require.Equal(t, false, ok)
}

func TestAPIPathsList(t *testing.T) {
	p, ok := newInstance("api: yes\n" +
		"paths:\n" +
		"  mypath:\n")
	require.Equal(t, true, ok)
	defer p.close()

	var out struct {
		Items map[string]interface{} `json:"items"`
	}
	err := httpRequest(http.MethodGet, "http://localhost:9997/paths/list", nil, &out)
	require.NoError(t, err)
	_, ok = out.Items["mypath"]
	require.Equal(t, true, ok)
}

func TestAPIRTSPSessionsList(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.close()

	track, err := gortsplib.NewTrackH264(96, []byte("123456"), []byte("123456"))
	require.NoError(t, err)

	source, err := gortsplib.DialPublish("rtsp://localhost:8554/mypath",
		gortsplib.Tracks{track})
	require.NoError(t, err)
	defer source.Close()

	var out struct {
		Items map[string]struct{} `json:"items"`
	}
	err = httpRequest(http.MethodGet, "http://localhost:9997/rtspsessions/list", nil, &out)
	require.NoError(t, err)
	require.Equal(t, 1, len(out.Items))
}

func TestAPIRTSPSessionsKick(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.close()

	track, err := gortsplib.NewTrackH264(96, []byte("123456"), []byte("123456"))
	require.NoError(t, err)

	source, err := gortsplib.DialPublish("rtsp://localhost:8554/mypath",
		gortsplib.Tracks{track})
	require.NoError(t, err)
	defer source.Close()

	var out1 struct {
		Items map[string]struct{} `json:"items"`
	}
	err = httpRequest(http.MethodGet, "http://localhost:9997/rtspsessions/list", nil, &out1)
	require.NoError(t, err)

	var firstID string
	for k := range out1.Items {
		firstID = k
	}

	err = httpRequest(http.MethodPost, "http://localhost:9997/rtspsessions/kick/"+firstID, nil, nil)
	require.NoError(t, err)

	var out2 struct {
		Items map[string]struct{} `json:"items"`
	}
	err = httpRequest(http.MethodGet, "http://localhost:9997/rtspsessions/list", nil, &out2)
	require.NoError(t, err)
	require.Equal(t, 0, len(out2.Items))
}

func TestAPIRTMPConnsList(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.close()

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.mkv",
		"-c", "copy",
		"-f", "flv",
		"rtmp://localhost:1935/test1/test2",
	})
	require.NoError(t, err)
	defer cnt1.close()

	var out struct {
		Items map[string]struct{} `json:"items"`
	}
	err = httpRequest(http.MethodGet, "http://localhost:9997/rtmpconns/list", nil, &out)
	require.NoError(t, err)
	require.Equal(t, 1, len(out.Items))
}

func TestAPIRTSPConnsKick(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.close()

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.mkv",
		"-c", "copy",
		"-f", "flv",
		"rtmp://localhost:1935/test1/test2",
	})
	require.NoError(t, err)
	defer cnt1.close()

	var out1 struct {
		Items map[string]struct{} `json:"items"`
	}
	err = httpRequest(http.MethodGet, "http://localhost:9997/rtmpconns/list", nil, &out1)
	require.NoError(t, err)

	var firstID string
	for k := range out1.Items {
		firstID = k
	}

	err = httpRequest(http.MethodPost, "http://localhost:9997/rtmpconns/kick/"+firstID, nil, nil)
	require.NoError(t, err)

	var out2 struct {
		Items map[string]struct{} `json:"items"`
	}
	err = httpRequest(http.MethodGet, "http://localhost:9997/rtmpconns/list", nil, &out2)
	require.NoError(t, err)
	require.Equal(t, 0, len(out2.Items))
}
