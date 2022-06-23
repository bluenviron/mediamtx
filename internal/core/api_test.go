package core

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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
	err := httpRequest(http.MethodGet, "http://localhost:9997/v1/config/get", nil, &out)
	require.NoError(t, err)
	require.Equal(t, true, out["api"])
}

func TestAPIConfigSet(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.close()

	err := httpRequest(http.MethodPost, "http://localhost:9997/v1/config/set", map[string]interface{}{
		"rtmpDisable": true,
		"readTimeout": "7s",
		"protocols":   []string{"tcp"},
	}, nil)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	var out map[string]interface{}
	err = httpRequest(http.MethodGet, "http://localhost:9997/v1/config/get", nil, &out)
	require.NoError(t, err)
	require.Equal(t, true, out["rtmpDisable"])
	require.Equal(t, "7s", out["readTimeout"])
	require.Equal(t, []interface{}{"tcp"}, out["protocols"])
}

func TestAPIConfigPathsAdd(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.close()

	err := httpRequest(http.MethodPost, "http://localhost:9997/v1/config/paths/add/my/path", map[string]interface{}{
		"source":         "rtsp://127.0.0.1:9999/mypath",
		"sourceOnDemand": true,
	}, nil)
	require.NoError(t, err)

	var out map[string]interface{}
	err = httpRequest(http.MethodGet, "http://localhost:9997/v1/config/get", nil, &out)
	require.NoError(t, err)
	require.Equal(t, "rtsp://127.0.0.1:9999/mypath",
		out["paths"].(map[string]interface{})["my/path"].(map[string]interface{})["source"])
}

func TestAPIConfigPathsEdit(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.close()

	err := httpRequest(http.MethodPost, "http://localhost:9997/v1/config/paths/add/my/path", map[string]interface{}{
		"source":         "rtsp://127.0.0.1:9999/mypath",
		"sourceOnDemand": true,
	}, nil)
	require.NoError(t, err)

	err = httpRequest(http.MethodPost, "http://localhost:9997/v1/config/paths/edit/my/path", map[string]interface{}{
		"source":         "rtsp://127.0.0.1:9998/mypath",
		"sourceOnDemand": true,
	}, nil)
	require.NoError(t, err)

	var out struct {
		Paths map[string]struct {
			Source string `json:"source"`
		} `json:"paths"`
	}
	err = httpRequest(http.MethodGet, "http://localhost:9997/v1/config/get", nil, &out)
	require.NoError(t, err)
	require.Equal(t, "rtsp://127.0.0.1:9998/mypath", out.Paths["my/path"].Source)
}

func TestAPIConfigPathsRemove(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.close()

	err := httpRequest(http.MethodPost, "http://localhost:9997/v1/config/paths/add/my/path", map[string]interface{}{
		"source":         "rtsp://127.0.0.1:9999/mypath",
		"sourceOnDemand": true,
	}, nil)
	require.NoError(t, err)

	err = httpRequest(http.MethodPost, "http://localhost:9997/v1/config/paths/remove/my/path", nil, nil)
	require.NoError(t, err)

	var out struct {
		Paths map[string]interface{} `json:"paths"`
	}
	err = httpRequest(http.MethodGet, "http://localhost:9997/v1/config/get", nil, &out)
	require.NoError(t, err)
	_, ok = out.Paths["my/path"]
	require.Equal(t, false, ok)
}

func TestAPIPathsList(t *testing.T) {
	serverCertFpath, err := writeTempFile(serverCert)
	require.NoError(t, err)
	defer os.Remove(serverCertFpath)

	serverKeyFpath, err := writeTempFile(serverKey)
	require.NoError(t, err)
	defer os.Remove(serverKeyFpath)

	p, ok := newInstance("api: yes\n" +
		"encryption: optional\n" +
		"serverCert: " + serverCertFpath + "\n" +
		"serverKey: " + serverKeyFpath + "\n" +
		"paths:\n" +
		"  mypath:\n")
	require.Equal(t, true, ok)
	defer p.close()

	var out struct {
		Items map[string]struct {
			Source struct {
				Type string `json:"type"`
			} `json:"source"`
		} `json:"items"`
	}
	err = httpRequest(http.MethodGet, "http://localhost:9997/v1/paths/list", nil, &out)
	require.NoError(t, err)
	_, ok = out.Items["mypath"]
	require.Equal(t, true, ok)

	track := &gortsplib.TrackH264{
		PayloadType: 96,
		SPS:         []byte{0x01, 0x02, 0x03, 0x04},
		PPS:         []byte{0x01, 0x02, 0x03, 0x04},
	}

	func() {
		source := gortsplib.Client{}

		err = source.StartPublishing("rtsp://localhost:8554/mypath",
			gortsplib.Tracks{track})
		require.NoError(t, err)
		defer source.Close()

		err = httpRequest(http.MethodGet, "http://localhost:9997/v1/paths/list", nil, &out)
		require.NoError(t, err)
		require.Equal(t, "rtspSession", out.Items["mypath"].Source.Type)
	}()

	func() {
		source := gortsplib.Client{
			TLSConfig: &tls.Config{InsecureSkipVerify: true},
		}

		err := source.StartPublishing("rtsps://localhost:8322/mypath",
			gortsplib.Tracks{track})
		require.NoError(t, err)
		defer source.Close()

		err = httpRequest(http.MethodGet, "http://localhost:9997/v1/paths/list", nil, &out)
		require.NoError(t, err)
		require.Equal(t, "rtspsSession", out.Items["mypath"].Source.Type)
	}()
}

func TestAPIList(t *testing.T) {
	serverCertFpath, err := writeTempFile(serverCert)
	require.NoError(t, err)
	defer os.Remove(serverCertFpath)

	serverKeyFpath, err := writeTempFile(serverKey)
	require.NoError(t, err)
	defer os.Remove(serverKeyFpath)

	for _, ca := range []string{
		"rtsp",
		"rtsps",
		"rtmp",
		"hls",
	} {
		t.Run(ca, func(t *testing.T) {
			conf := "api: yes\n"

			if ca == "rtsps" {
				conf += "protocols: [tcp]\n" +
					"encryption: strict\n" +
					"serverCert: " + serverCertFpath + "\n" +
					"serverKey: " + serverKeyFpath + "\n"
			}

			conf += "paths:\n" +
				"  all:\n"

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.close()

			track := &gortsplib.TrackH264{
				PayloadType: 96,
				SPS:         []byte{0x01, 0x02, 0x03, 0x04},
				PPS:         []byte{0x01, 0x02, 0x03, 0x04},
			}

			switch ca {
			case "rtsp":
				source := gortsplib.Client{}

				err := source.StartPublishing("rtsp://localhost:8554/mypath",
					gortsplib.Tracks{track})
				require.NoError(t, err)
				defer source.Close()

			case "rtsps":
				source := gortsplib.Client{
					TLSConfig: &tls.Config{InsecureSkipVerify: true},
				}

				err := source.StartPublishing("rtsps://localhost:8322/mypath",
					gortsplib.Tracks{track})
				require.NoError(t, err)
				defer source.Close()

			case "rtmp":
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

			case "hls":
				source := gortsplib.Client{}

				err := source.StartPublishing("rtsp://localhost:8554/mypath",
					gortsplib.Tracks{track})
				require.NoError(t, err)
				defer source.Close()

				func() {
					res, err := http.Get("http://localhost:8888/mypath/index.m3u8")
					require.NoError(t, err)
					defer res.Body.Close()
					require.Equal(t, 200, res.StatusCode)
				}()
			}

			switch ca {
			case "rtsp", "rtsps", "rtmp":
				var pa string
				switch ca {
				case "rtsp":
					pa = "rtspsessions"

				case "rtsps":
					pa = "rtspssessions"

				case "rtmp":
					pa = "rtmpconns"
				}

				var out struct {
					Items map[string]struct {
						State string `json:"state"`
					} `json:"items"`
				}
				err = httpRequest(http.MethodGet, "http://localhost:9997/v1/"+pa+"/list", nil, &out)
				require.NoError(t, err)

				var firstID string
				for k := range out.Items {
					firstID = k
				}

				require.Equal(t, "publish", out.Items[firstID].State)

			case "hls":
				var out struct {
					Items map[string]struct {
						LastRequest string `json:"lastRequest"`
					} `json:"items"`
				}
				err = httpRequest(http.MethodGet, "http://localhost:9997/v1/hlsmuxers/list", nil, &out)
				require.NoError(t, err)

				var firstID string
				for k := range out.Items {
					firstID = k
				}

				require.NotEqual(t, "", out.Items[firstID].LastRequest)
			}
		})
	}
}

func TestAPIKick(t *testing.T) {
	serverCertFpath, err := writeTempFile(serverCert)
	require.NoError(t, err)
	defer os.Remove(serverCertFpath)

	serverKeyFpath, err := writeTempFile(serverKey)
	require.NoError(t, err)
	defer os.Remove(serverKeyFpath)

	for _, ca := range []string{
		"rtsp",
		"rtsps",
		"rtmp",
	} {
		t.Run(ca, func(t *testing.T) {
			conf := "api: yes\n"

			if ca == "rtsps" {
				conf += "protocols: [tcp]\n" +
					"encryption: strict\n" +
					"serverCert: " + serverCertFpath + "\n" +
					"serverKey: " + serverKeyFpath + "\n"
			}

			conf += "paths:\n" +
				"  all:\n"

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.close()

			track := &gortsplib.TrackH264{
				PayloadType: 96,
				SPS:         []byte{0x01, 0x02, 0x03, 0x04},
				PPS:         []byte{0x01, 0x02, 0x03, 0x04},
			}

			switch ca {
			case "rtsp":
				source := gortsplib.Client{}

				err := source.StartPublishing("rtsp://localhost:8554/mypath",
					gortsplib.Tracks{track})
				require.NoError(t, err)
				defer source.Close()

			case "rtsps":
				source := gortsplib.Client{
					TLSConfig: &tls.Config{InsecureSkipVerify: true},
				}

				err := source.StartPublishing("rtsps://localhost:8322/mypath",
					gortsplib.Tracks{track})
				require.NoError(t, err)
				defer source.Close()

			case "rtmp":
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
			}

			var pa string
			switch ca {
			case "rtsp":
				pa = "rtspsessions"

			case "rtsps":
				pa = "rtspssessions"

			case "rtmp":
				pa = "rtmpconns"
			}

			var out1 struct {
				Items map[string]struct{} `json:"items"`
			}
			err = httpRequest(http.MethodGet, "http://localhost:9997/v1/"+pa+"/list", nil, &out1)
			require.NoError(t, err)

			var firstID string
			for k := range out1.Items {
				firstID = k
			}

			err = httpRequest(http.MethodPost, "http://localhost:9997/v1/"+pa+"/kick/"+firstID, nil, nil)
			require.NoError(t, err)

			var out2 struct {
				Items map[string]struct{} `json:"items"`
			}
			err = httpRequest(http.MethodGet, "http://localhost:9997/v1/"+pa+"/list", nil, &out2)
			require.NoError(t, err)
			require.Equal(t, 0, len(out2.Items))
		})
	}
}
