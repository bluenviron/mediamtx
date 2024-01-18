package core

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
	srt "github.com/datarhei/gosrt"
	"github.com/google/uuid"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp"
	"github.com/bluenviron/mediamtx/internal/protocols/webrtc"
)

var testFormatH264 = &format.H264{
	PayloadTyp: 96,
	SPS: []byte{ // 1920x1080 baseline
		0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
		0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
		0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
	},
	PPS:               []byte{0x08, 0x06, 0x07, 0x08},
	PacketizationMode: 1,
}

var testMediaH264 = &description.Media{
	Type:    description.MediaTypeVideo,
	Formats: []format.Format{testFormatH264},
}

var testMediaAAC = &description.Media{
	Type: description.MediaTypeAudio,
	Formats: []format.Format{&format.MPEG4Audio{
		PayloadTyp: 96,
		Config: &mpeg4audio.Config{
			Type:         2,
			SampleRate:   44100,
			ChannelCount: 2,
		},
		SizeLength:       13,
		IndexLength:      3,
		IndexDeltaLength: 3,
	}},
}

func checkClose(t *testing.T, closeFunc func() error) {
	require.NoError(t, closeFunc())
}

func checkError(t *testing.T, msg string, body io.Reader) {
	var resErr map[string]interface{}
	err := json.NewDecoder(body).Decode(&resErr)
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{"error": msg}, resErr)
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

func TestAPIConfigGlobalGet(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.Close()

	hc := &http.Client{Transport: &http.Transport{}}

	var out map[string]interface{}
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/global/get", nil, &out)
	require.Equal(t, true, out["api"])
}

func TestAPIConfigGlobalPatch(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.Close()

	hc := &http.Client{Transport: &http.Transport{}}

	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/global/patch", map[string]interface{}{
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

func TestAPIConfigGlobalPatchUnknownField(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.Close()

	b := map[string]interface{}{
		"test": "asd",
	}

	byts, err := json.Marshal(b)
	require.NoError(t, err)

	hc := &http.Client{Transport: &http.Transport{}}

	func() {
		req, err := http.NewRequest(http.MethodPatch, "http://localhost:9997/v3/config/global/patch", bytes.NewReader(byts))
		require.NoError(t, err)

		res, err := hc.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()

		require.Equal(t, http.StatusBadRequest, res.StatusCode)
		checkError(t, "json: unknown field \"test\"", res.Body)
	}()
}

func TestAPIConfigPathDefaultsGet(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.Close()

	hc := &http.Client{Transport: &http.Transport{}}

	var out map[string]interface{}
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/pathdefaults/get", nil, &out)
	require.Equal(t, "publisher", out["source"])
}

func TestAPIConfigPathDefaultsPatch(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.Close()

	hc := &http.Client{Transport: &http.Transport{}}

	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/pathdefaults/patch", map[string]interface{}{
		"readUser": "myuser",
		"readPass": "mypass",
	}, nil)

	time.Sleep(500 * time.Millisecond)

	var out map[string]interface{}
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/pathdefaults/get", nil, &out)
	require.Equal(t, "myuser", out["readUser"])
	require.Equal(t, "mypass", out["readPass"])
}

func TestAPIConfigPathsList(t *testing.T) {
	p, ok := newInstance("api: yes\n" +
		"paths:\n" +
		"  path1:\n" +
		"    readUser: myuser1\n" +
		"    readPass: mypass1\n" +
		"  path2:\n" +
		"    readUser: myuser2\n" +
		"    readPass: mypass2\n")
	require.Equal(t, true, ok)
	defer p.Close()

	type pathConfig map[string]interface{}

	type listRes struct {
		ItemCount int          `json:"itemCount"`
		PageCount int          `json:"pageCount"`
		Items     []pathConfig `json:"items"`
	}

	hc := &http.Client{Transport: &http.Transport{}}

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

func TestAPIConfigPathsGet(t *testing.T) {
	p, ok := newInstance("api: yes\n" +
		"paths:\n" +
		"  my/path:\n" +
		"    readUser: myuser\n" +
		"    readPass: mypass\n")
	require.Equal(t, true, ok)
	defer p.Close()

	hc := &http.Client{Transport: &http.Transport{}}

	var out map[string]interface{}
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/config/paths/get/my/path", nil, &out)
	require.Equal(t, "my/path", out["name"])
	require.Equal(t, "myuser", out["readUser"])
}

func TestAPIConfigPathsAdd(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.Close()

	hc := &http.Client{Transport: &http.Transport{}}

	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/config/paths/add/my/path", map[string]interface{}{
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

func TestAPIConfigPathsAddUnknownField(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.Close()

	b := map[string]interface{}{
		"test": "asd",
	}

	byts, err := json.Marshal(b)
	require.NoError(t, err)

	hc := &http.Client{Transport: &http.Transport{}}

	func() {
		req, err := http.NewRequest(http.MethodPost,
			"http://localhost:9997/v3/config/paths/add/my/path", bytes.NewReader(byts))
		require.NoError(t, err)

		res, err := hc.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()

		require.Equal(t, http.StatusBadRequest, res.StatusCode)
		checkError(t, "json: unknown field \"test\"", res.Body)
	}()
}

func TestAPIConfigPathsPatch(t *testing.T) { //nolint:dupl
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.Close()

	hc := &http.Client{Transport: &http.Transport{}}

	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/config/paths/add/my/path", map[string]interface{}{
		"source":                   "rtsp://127.0.0.1:9999/mypath",
		"sourceOnDemand":           true,
		"disablePublisherOverride": true, // test setting a deprecated parameter
		"rpiCameraVFlip":           true,
	}, nil)

	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/paths/patch/my/path", map[string]interface{}{
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

func TestAPIConfigPathsReplace(t *testing.T) { //nolint:dupl
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.Close()

	hc := &http.Client{Transport: &http.Transport{}}

	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/config/paths/add/my/path", map[string]interface{}{
		"source":                   "rtsp://127.0.0.1:9999/mypath",
		"sourceOnDemand":           true,
		"disablePublisherOverride": true, // test setting a deprecated parameter
		"rpiCameraVFlip":           true,
	}, nil)

	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/config/paths/replace/my/path", map[string]interface{}{
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

func TestAPIConfigPathsDelete(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.Close()

	hc := &http.Client{Transport: &http.Transport{}}

	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/config/paths/add/my/path", map[string]interface{}{
		"source":         "rtsp://127.0.0.1:9999/mypath",
		"sourceOnDemand": true,
	}, nil)

	httpRequest(t, hc, http.MethodDelete, "http://localhost:9997/v3/config/paths/delete/my/path", nil, nil)

	func() {
		req, err := http.NewRequest(http.MethodGet, "http://localhost:9997/v3/config/paths/get/my/path", nil)
		require.NoError(t, err)

		res, err := hc.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()

		require.Equal(t, http.StatusNotFound, res.StatusCode)
		checkError(t, "path configuration not found", res.Body)
	}()
}

func TestAPIPathsList(t *testing.T) {
	type pathSource struct {
		Type string `json:"type"`
	}

	type path struct {
		Name          string     `json:"name"`
		Source        pathSource `json:"source"`
		Ready         bool       `json:"ready"`
		Tracks        []string   `json:"tracks"`
		BytesReceived uint64     `json:"bytesReceived"`
		BytesSent     uint64     `json:"bytesSent"`
	}

	type pathList struct {
		ItemCount int    `json:"itemCount"`
		PageCount int    `json:"pageCount"`
		Items     []path `json:"items"`
	}

	t.Run("rtsp session", func(t *testing.T) {
		p, ok := newInstance("api: yes\n" +
			"paths:\n" +
			"  mypath:\n")
		require.Equal(t, true, ok)
		defer p.Close()

		hc := &http.Client{Transport: &http.Transport{}}

		media0 := testMediaH264

		source := gortsplib.Client{}
		err := source.StartRecording(
			"rtsp://localhost:8554/mypath",
			&description.Session{Medias: []*description.Media{
				media0,
				testMediaAAC,
			}})
		require.NoError(t, err)
		defer source.Close()

		err = source.WritePacketRTP(media0, &rtp.Packet{
			Header: rtp.Header{
				Version:     2,
				PayloadType: 96,
			},
			Payload: []byte{5, 1, 2, 3, 4},
		})
		require.NoError(t, err)

		var out pathList
		httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/list", nil, &out)
		require.Equal(t, pathList{
			ItemCount: 1,
			PageCount: 1,
			Items: []path{{
				Name: "mypath",
				Source: pathSource{
					Type: "rtspSession",
				},
				Ready:         true,
				Tracks:        []string{"H264", "MPEG-4 Audio"},
				BytesReceived: 17,
			}},
		}, out)
	})

	t.Run("rtsps session", func(t *testing.T) {
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
		defer p.Close()

		hc := &http.Client{Transport: &http.Transport{}}

		source := gortsplib.Client{TLSConfig: &tls.Config{InsecureSkipVerify: true}}
		err = source.StartRecording("rtsps://localhost:8322/mypath",
			&description.Session{Medias: []*description.Media{
				testMediaH264,
				testMediaAAC,
			}})
		require.NoError(t, err)
		defer source.Close()

		var out pathList
		httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/list", nil, &out)
		require.Equal(t, pathList{
			ItemCount: 1,
			PageCount: 1,
			Items: []path{{
				Name: "mypath",
				Source: pathSource{
					Type: "rtspsSession",
				},
				Ready:  true,
				Tracks: []string{"H264", "MPEG-4 Audio"},
			}},
		}, out)
	})

	t.Run("rtsp source", func(t *testing.T) {
		p, ok := newInstance("api: yes\n" +
			"paths:\n" +
			"  mypath:\n" +
			"    source: rtsp://127.0.0.1:1234/mypath\n" +
			"    sourceOnDemand: yes\n")
		require.Equal(t, true, ok)
		defer p.Close()

		hc := &http.Client{Transport: &http.Transport{}}

		var out pathList
		httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/list", nil, &out)
		require.Equal(t, pathList{
			ItemCount: 1,
			PageCount: 1,
			Items: []path{{
				Name: "mypath",
				Source: pathSource{
					Type: "rtspSource",
				},
				Ready:  false,
				Tracks: []string{},
			}},
		}, out)
	})

	t.Run("rtmp source", func(t *testing.T) {
		p, ok := newInstance("api: yes\n" +
			"paths:\n" +
			"  mypath:\n" +
			"    source: rtmp://127.0.0.1:1234/mypath\n" +
			"    sourceOnDemand: yes\n")
		require.Equal(t, true, ok)
		defer p.Close()

		hc := &http.Client{Transport: &http.Transport{}}

		var out pathList
		httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/list", nil, &out)
		require.Equal(t, pathList{
			ItemCount: 1,
			PageCount: 1,
			Items: []path{{
				Name: "mypath",
				Source: pathSource{
					Type: "rtmpSource",
				},
				Ready:  false,
				Tracks: []string{},
			}},
		}, out)
	})

	t.Run("hls source", func(t *testing.T) {
		p, ok := newInstance("api: yes\n" +
			"paths:\n" +
			"  mypath:\n" +
			"    source: http://127.0.0.1:1234/mypath\n" +
			"    sourceOnDemand: yes\n")
		require.Equal(t, true, ok)
		defer p.Close()

		hc := &http.Client{Transport: &http.Transport{}}

		var out pathList
		httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/list", nil, &out)
		require.Equal(t, pathList{
			ItemCount: 1,
			PageCount: 1,
			Items: []path{{
				Name: "mypath",
				Source: pathSource{
					Type: "hlsSource",
				},
				Ready:  false,
				Tracks: []string{},
			}},
		}, out)
	})
}

func TestAPIPathsGet(t *testing.T) {
	p, ok := newInstance("api: yes\n" +
		"paths:\n" +
		"  all_others:\n")
	require.Equal(t, true, ok)
	defer p.Close()

	hc := &http.Client{Transport: &http.Transport{}}

	for _, ca := range []string{"ok", "ok-nested", "not found"} {
		t.Run(ca, func(t *testing.T) {
			type pathSource struct {
				Type string `json:"type"`
			}

			type path struct {
				Name          string     `json:"name"`
				Source        pathSource `json:"source"`
				Ready         bool       `json:"Ready"`
				Tracks        []string   `json:"tracks"`
				BytesReceived uint64     `json:"bytesReceived"`
				BytesSent     uint64     `json:"bytesSent"`
			}

			var pathName string

			switch ca {
			case "ok":
				pathName = "mypath"
			case "ok-nested":
				pathName = "my/nested/path"
			case "not found":
				pathName = "nonexisting"
			}

			if ca == "ok" || ca == "ok-nested" {
				source := gortsplib.Client{}
				err := source.StartRecording("rtsp://localhost:8554/"+pathName,
					&description.Session{Medias: []*description.Media{testMediaH264}})
				require.NoError(t, err)
				defer source.Close()

				var out path
				httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/get/"+pathName, nil, &out)
				require.Equal(t, path{
					Name: pathName,
					Source: pathSource{
						Type: "rtspSession",
					},
					Ready:  true,
					Tracks: []string{"H264"},
				}, out)
			} else {
				res, err := hc.Get("http://localhost:9997/v3/paths/get/" + pathName)
				require.NoError(t, err)
				defer res.Body.Close()

				require.Equal(t, http.StatusNotFound, res.StatusCode)
				checkError(t, "path not found", res.Body)
			}
		})
	}
}

func TestAPIProtocolList(t *testing.T) {
	serverCertFpath, err := writeTempFile(serverCert)
	require.NoError(t, err)
	defer os.Remove(serverCertFpath)

	serverKeyFpath, err := writeTempFile(serverKey)
	require.NoError(t, err)
	defer os.Remove(serverKeyFpath)

	for _, ca := range []string{
		"rtsp conns",
		"rtsp sessions",
		"rtsps conns",
		"rtsps sessions",
		"rtmp",
		"rtmps",
		"hls",
		"webrtc",
		"srt",
	} {
		t.Run(ca, func(t *testing.T) {
			conf := "api: yes\n"

			switch ca {
			case "rtsps conns", "rtsps sessions":
				conf += "protocols: [tcp]\n" +
					"encryption: strict\n" +
					"serverCert: " + serverCertFpath + "\n" +
					"serverKey: " + serverKeyFpath + "\n"

			case "rtmps":
				conf += "rtmpEncryption: strict\n" +
					"rtmpServerCert: " + serverCertFpath + "\n" +
					"rtmpServerKey: " + serverKeyFpath + "\n"
			}

			conf += "paths:\n" +
				"  all_others:\n"

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.Close()

			hc := &http.Client{Transport: &http.Transport{}}

			medi := testMediaH264

			switch ca { //nolint:dupl
			case "rtsp conns", "rtsp sessions":
				source := gortsplib.Client{}

				err := source.StartRecording("rtsp://localhost:8554/mypath?key=val",
					&description.Session{Medias: []*description.Media{medi}})
				require.NoError(t, err)
				defer source.Close()

			case "rtsps conns", "rtsps sessions":
				source := gortsplib.Client{
					TLSConfig: &tls.Config{InsecureSkipVerify: true},
				}

				err := source.StartRecording("rtsps://localhost:8322/mypath?key=val",
					&description.Session{Medias: []*description.Media{medi}})
				require.NoError(t, err)
				defer source.Close()

			case "rtmp", "rtmps":
				var port string
				if ca == "rtmp" {
					port = "1935"
				} else {
					port = "1936"
				}

				u, err := url.Parse("rtmp://127.0.0.1:" + port + "/mypath?key=val")
				require.NoError(t, err)

				nconn, err := func() (net.Conn, error) {
					if ca == "rtmp" {
						return net.Dial("tcp", u.Host)
					}
					return tls.Dial("tcp", u.Host, &tls.Config{InsecureSkipVerify: true})
				}()
				require.NoError(t, err)
				defer nconn.Close()

				conn, err := rtmp.NewClientConn(nconn, u, true)
				require.NoError(t, err)

				_, err = rtmp.NewWriter(conn, testFormatH264, nil)
				require.NoError(t, err)

				time.Sleep(500 * time.Millisecond)

			case "hls":
				source := gortsplib.Client{}
				err := source.StartRecording("rtsp://localhost:8554/mypath",
					&description.Session{Medias: []*description.Media{medi}})
				require.NoError(t, err)
				defer source.Close()

				go func() {
					time.Sleep(500 * time.Millisecond)

					for i := 0; i < 3; i++ {
						/*source.WritePacketRTP(medi, &rtp.Packet{
							Header: rtp.Header{
								Version:        2,
								Marker:         true,
								PayloadType:    96,
								SequenceNumber: 123 + uint16(i),
								Timestamp:      45343 + uint32(i)*90000,
								SSRC:           563423,
							},
							Payload: []byte{
								testSPS,
								0x05,
							},
						})

						[]byte{ // 1920x1080 baseline
							0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
							0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
							0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
						},*/

						err := source.WritePacketRTP(medi, &rtp.Packet{
							Header: rtp.Header{
								Version:        2,
								Marker:         true,
								PayloadType:    96,
								SequenceNumber: 123 + uint16(i),
								Timestamp:      45343 + uint32(i)*90000,
								SSRC:           563423,
							},
							Payload: []byte{
								// testSPS,
								0x05,
							},
						})
						require.NoError(t, err)
					}
				}()

				func() {
					res, err := hc.Get("http://localhost:8888/mypath/index.m3u8")
					require.NoError(t, err)
					defer res.Body.Close()
					require.Equal(t, 200, res.StatusCode)
				}()

			case "webrtc":
				source := gortsplib.Client{}
				err := source.StartRecording("rtsp://localhost:8554/mypath",
					&description.Session{Medias: []*description.Media{medi}})
				require.NoError(t, err)
				defer source.Close()

				u, err := url.Parse("http://localhost:8889/mypath/whep?key=val")
				require.NoError(t, err)

				go func() {
					time.Sleep(500 * time.Millisecond)

					err := source.WritePacketRTP(medi, &rtp.Packet{
						Header: rtp.Header{
							Version:        2,
							Marker:         true,
							PayloadType:    96,
							SequenceNumber: 123,
							Timestamp:      45343,
							SSRC:           563423,
						},
						Payload: []byte{5, 1, 2, 3, 4},
					})
					require.NoError(t, err)
				}()

				c := &webrtc.WHIPClient{
					HTTPClient: hc,
					URL:        u,
				}

				_, err = c.Read(context.Background())
				require.NoError(t, err)
				defer checkClose(t, c.Close)

			case "srt":
				conf := srt.DefaultConfig()
				conf.StreamId = "publish:mypath:::key=val"

				conn, err := srt.Dial("srt", "localhost:8890", conf)
				require.NoError(t, err)
				defer conn.Close()

				track := &mpegts.Track{
					Codec: &mpegts.CodecH264{},
				}

				bw := bufio.NewWriter(conn)
				w := mpegts.NewWriter(bw, []*mpegts.Track{track})
				require.NoError(t, err)

				err = w.WriteH26x(track, 0, 0, true, [][]byte{{1}})
				require.NoError(t, err)

				err = bw.Flush()
				require.NoError(t, err)

				time.Sleep(500 * time.Millisecond)
			}

			switch ca {
			case "rtsp conns", "rtsp sessions", "rtsps conns", "rtsps sessions", "rtmp", "rtmps", "srt":
				var pa string
				switch ca {
				case "rtsp conns":
					pa = "rtspconns"

				case "rtsp sessions":
					pa = "rtspsessions"

				case "rtsps conns":
					pa = "rtspsconns"

				case "rtsps sessions":
					pa = "rtspssessions"

				case "rtmp":
					pa = "rtmpconns"

				case "rtmps":
					pa = "rtmpsconns"

				case "srt":
					pa = "srtconns"
				}

				type item struct {
					State string `json:"state"`
					Path  string `json:"path"`
					Query string `json:"query"`
				}

				var out struct {
					ItemCount int    `json:"itemCount"`
					Items     []item `json:"items"`
				}
				httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/"+pa+"/list", nil, &out)

				if ca != "rtsp conns" && ca != "rtsps conns" {
					require.Equal(t, item{
						State: "publish",
						Path:  "mypath",
						Query: "key=val",
					}, out.Items[0])
				}

			case "hls":
				type item struct {
					Created     string `json:"created"`
					LastRequest string `json:"lastRequest"`
				}

				var out struct {
					ItemCount int    `json:"itemCount"`
					Items     []item `json:"items"`
				}
				httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/hlsmuxers/list", nil, &out)

				s := fmt.Sprintf("^%d-", time.Now().Year())
				require.Regexp(t, s, out.Items[0].Created)
				require.Regexp(t, s, out.Items[0].LastRequest)

			case "webrtc":
				type item struct {
					PeerConnectionEstablished bool   `json:"peerConnectionEstablished"`
					State                     string `json:"state"`
					Path                      string `json:"path"`
					Query                     string `json:"query"`
				}

				var out struct {
					ItemCount int    `json:"itemCount"`
					Items     []item `json:"items"`
				}
				httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/webrtcsessions/list", nil, &out)

				require.Equal(t, item{
					PeerConnectionEstablished: true,
					State:                     "read",
					Path:                      "mypath",
					Query:                     "key=val",
				}, out.Items[0])
			}
		})
	}
}

func TestAPIProtocolGet(t *testing.T) {
	serverCertFpath, err := writeTempFile(serverCert)
	require.NoError(t, err)
	defer os.Remove(serverCertFpath)

	serverKeyFpath, err := writeTempFile(serverKey)
	require.NoError(t, err)
	defer os.Remove(serverKeyFpath)

	for _, ca := range []string{
		"rtsp conns",
		"rtsp sessions",
		"rtsps conns",
		"rtsps sessions",
		"rtmp",
		"rtmps",
		"hls",
		"webrtc",
		"srt",
	} {
		t.Run(ca, func(t *testing.T) {
			conf := "api: yes\n"

			switch ca {
			case "rtsps conns", "rtsps sessions":
				conf += "protocols: [tcp]\n" +
					"encryption: strict\n" +
					"serverCert: " + serverCertFpath + "\n" +
					"serverKey: " + serverKeyFpath + "\n"

			case "rtmps":
				conf += "rtmpEncryption: strict\n" +
					"rtmpServerCert: " + serverCertFpath + "\n" +
					"rtmpServerKey: " + serverKeyFpath + "\n"
			}

			conf += "paths:\n" +
				"  all_others:\n"

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.Close()

			hc := &http.Client{Transport: &http.Transport{}}

			medi := testMediaH264

			switch ca { //nolint:dupl
			case "rtsp conns", "rtsp sessions":
				source := gortsplib.Client{}

				err := source.StartRecording("rtsp://localhost:8554/mypath",
					&description.Session{Medias: []*description.Media{medi}})
				require.NoError(t, err)
				defer source.Close()

			case "rtsps conns", "rtsps sessions":
				source := gortsplib.Client{
					TLSConfig: &tls.Config{InsecureSkipVerify: true},
				}

				err := source.StartRecording("rtsps://localhost:8322/mypath",
					&description.Session{Medias: []*description.Media{medi}})
				require.NoError(t, err)
				defer source.Close()

			case "rtmp", "rtmps":
				var port string
				if ca == "rtmp" {
					port = "1935"
				} else {
					port = "1936"
				}

				u, err := url.Parse("rtmp://127.0.0.1:" + port + "/mypath")
				require.NoError(t, err)

				nconn, err := func() (net.Conn, error) {
					if ca == "rtmp" {
						return net.Dial("tcp", u.Host)
					}
					return tls.Dial("tcp", u.Host, &tls.Config{InsecureSkipVerify: true})
				}()
				require.NoError(t, err)
				defer nconn.Close()

				conn, err := rtmp.NewClientConn(nconn, u, true)
				require.NoError(t, err)

				_, err = rtmp.NewWriter(conn, testFormatH264, nil)
				require.NoError(t, err)

				time.Sleep(500 * time.Millisecond)

			case "hls":
				source := gortsplib.Client{}
				err := source.StartRecording("rtsp://localhost:8554/mypath",
					&description.Session{Medias: []*description.Media{medi}})
				require.NoError(t, err)
				defer source.Close()

				go func() {
					time.Sleep(500 * time.Millisecond)

					for i := 0; i < 3; i++ {
						/*source.WritePacketRTP(medi, &rtp.Packet{
							Header: rtp.Header{
								Version:        2,
								Marker:         true,
								PayloadType:    96,
								SequenceNumber: 123 + uint16(i),
								Timestamp:      45343 + uint32(i)*90000,
								SSRC:           563423,
							},
							Payload: []byte{
								testSPS,
								0x05,
							},
						})

						[]byte{ // 1920x1080 baseline
							0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
							0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
							0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
						},*/

						err := source.WritePacketRTP(medi, &rtp.Packet{
							Header: rtp.Header{
								Version:        2,
								Marker:         true,
								PayloadType:    96,
								SequenceNumber: 123 + uint16(i),
								Timestamp:      45343 + uint32(i)*90000,
								SSRC:           563423,
							},
							Payload: []byte{
								// testSPS,
								0x05,
							},
						})
						require.NoError(t, err)
					}
				}()

				func() {
					res, err := hc.Get("http://localhost:8888/mypath/index.m3u8")
					require.NoError(t, err)
					defer res.Body.Close()
					require.Equal(t, 200, res.StatusCode)
				}()

			case "webrtc":
				source := gortsplib.Client{}
				err := source.StartRecording("rtsp://localhost:8554/mypath",
					&description.Session{Medias: []*description.Media{medi}})
				require.NoError(t, err)
				defer source.Close()

				u, err := url.Parse("http://localhost:8889/mypath/whep")
				require.NoError(t, err)

				go func() {
					time.Sleep(500 * time.Millisecond)

					err := source.WritePacketRTP(medi, &rtp.Packet{
						Header: rtp.Header{
							Version:        2,
							Marker:         true,
							PayloadType:    96,
							SequenceNumber: 123,
							Timestamp:      45343,
							SSRC:           563423,
						},
						Payload: []byte{5, 1, 2, 3, 4},
					})
					require.NoError(t, err)
				}()

				c := &webrtc.WHIPClient{
					HTTPClient: hc,
					URL:        u,
				}

				_, err = c.Read(context.Background())
				require.NoError(t, err)
				defer checkClose(t, c.Close)

			case "srt":
				conf := srt.DefaultConfig()
				conf.StreamId = "publish:mypath"

				conn, err := srt.Dial("srt", "localhost:8890", conf)
				require.NoError(t, err)
				defer conn.Close()

				track := &mpegts.Track{
					Codec: &mpegts.CodecH264{},
				}

				bw := bufio.NewWriter(conn)
				w := mpegts.NewWriter(bw, []*mpegts.Track{track})
				require.NoError(t, err)

				err = w.WriteH26x(track, 0, 0, true, [][]byte{{1}})
				require.NoError(t, err)

				err = bw.Flush()
				require.NoError(t, err)

				time.Sleep(500 * time.Millisecond)
			}

			switch ca {
			case "rtsp conns", "rtsp sessions", "rtsps conns", "rtsps sessions", "rtmp", "rtmps", "srt":
				var pa string
				switch ca {
				case "rtsp conns":
					pa = "rtspconns"

				case "rtsp sessions":
					pa = "rtspsessions"

				case "rtsps conns":
					pa = "rtspsconns"

				case "rtsps sessions":
					pa = "rtspssessions"

				case "rtmp":
					pa = "rtmpconns"

				case "rtmps":
					pa = "rtmpsconns"

				case "srt":
					pa = "srtconns"
				}

				type item struct {
					ID    string `json:"id"`
					State string `json:"state"`
				}

				var out1 struct {
					Items []item `json:"items"`
				}
				httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/"+pa+"/list", nil, &out1)

				if ca != "rtsp conns" && ca != "rtsps conns" {
					require.Equal(t, "publish", out1.Items[0].State)
				}

				var out2 item
				httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/"+pa+"/get/"+out1.Items[0].ID, nil, &out2)

				if ca != "rtsp conns" && ca != "rtsps conns" {
					require.Equal(t, "publish", out2.State)
				}

			case "hls":
				type item struct {
					Created     string `json:"created"`
					LastRequest string `json:"lastRequest"`
				}

				var out item
				httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/hlsmuxers/get/mypath", nil, &out)

				s := fmt.Sprintf("^%d-", time.Now().Year())
				require.Regexp(t, s, out.Created)
				require.Regexp(t, s, out.LastRequest)

			case "webrtc":
				type item struct {
					ID                        string    `json:"id"`
					Created                   time.Time `json:"created"`
					RemoteAddr                string    `json:"remoteAddr"`
					PeerConnectionEstablished bool      `json:"peerConnectionEstablished"`
					LocalCandidate            string    `json:"localCandidate"`
					RemoteCandidate           string    `json:"remoteCandidate"`
					BytesReceived             uint64    `json:"bytesReceived"`
					BytesSent                 uint64    `json:"bytesSent"`
				}

				var out1 struct {
					Items []item `json:"items"`
				}
				httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/webrtcsessions/list", nil, &out1)

				var out2 item
				httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/webrtcsessions/get/"+out1.Items[0].ID, nil, &out2)

				require.Equal(t, true, out2.PeerConnectionEstablished)
			}
		})
	}
}

func TestAPIProtocolGetNotFound(t *testing.T) {
	serverCertFpath, err := writeTempFile(serverCert)
	require.NoError(t, err)
	defer os.Remove(serverCertFpath)

	serverKeyFpath, err := writeTempFile(serverKey)
	require.NoError(t, err)
	defer os.Remove(serverKeyFpath)

	for _, ca := range []string{
		"rtsp conns",
		"rtsp sessions",
		"rtsps conns",
		"rtsps sessions",
		"rtmp",
		"rtmps",
		"hls",
		"webrtc",
		"srt",
	} {
		t.Run(ca, func(t *testing.T) {
			conf := "api: yes\n"

			switch ca {
			case "rtsps conns", "rtsps sessions":
				conf += "protocols: [tcp]\n" +
					"encryption: strict\n" +
					"serverCert: " + serverCertFpath + "\n" +
					"serverKey: " + serverKeyFpath + "\n"

			case "rtmps":
				conf += "rtmpEncryption: strict\n" +
					"rtmpServerCert: " + serverCertFpath + "\n" +
					"rtmpServerKey: " + serverKeyFpath + "\n"
			}

			conf += "paths:\n" +
				"  all_others:\n"

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.Close()

			hc := &http.Client{Transport: &http.Transport{}}

			var pa string
			switch ca {
			case "rtsp conns":
				pa = "rtspconns"

			case "rtsp sessions":
				pa = "rtspsessions"

			case "rtsps conns":
				pa = "rtspsconns"

			case "rtsps sessions":
				pa = "rtspssessions"

			case "rtmp":
				pa = "rtmpconns"

			case "rtmps":
				pa = "rtmpsconns"

			case "hls":
				pa = "hlsmuxers"

			case "webrtc":
				pa = "webrtcsessions"

			case "srt":
				pa = "srtconns"
			}

			func() {
				req, err := http.NewRequest(http.MethodGet, "http://localhost:9997/v3/"+pa+"/get/"+uuid.New().String(), nil)
				require.NoError(t, err)

				res, err := hc.Do(req)
				require.NoError(t, err)
				defer res.Body.Close()

				require.Equal(t, http.StatusNotFound, res.StatusCode)

				switch ca {
				case "rtsp conns", "rtsps conns", "rtmp", "rtmps", "srt":
					checkError(t, "connection not found", res.Body)

				case "rtsp sessions", "rtsps sessions", "webrtc":
					checkError(t, "session not found", res.Body)

				case "hls":
					checkError(t, "muxer not found", res.Body)
				}
			}()
		})
	}
}

func TestAPIProtocolKick(t *testing.T) {
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
		"webrtc",
		"srt",
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
				"  all_others:\n"

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.Close()

			hc := &http.Client{Transport: &http.Transport{}}

			medi := testMediaH264

			switch ca {
			case "rtsp":
				source := gortsplib.Client{}

				err := source.StartRecording("rtsp://localhost:8554/mypath",
					&description.Session{Medias: []*description.Media{medi}})
				require.NoError(t, err)
				defer source.Close()

			case "rtsps":
				source := gortsplib.Client{
					TLSConfig: &tls.Config{InsecureSkipVerify: true},
				}

				err := source.StartRecording("rtsps://localhost:8322/mypath",
					&description.Session{Medias: []*description.Media{medi}})
				require.NoError(t, err)
				defer source.Close()

			case "rtmp":
				u, err := url.Parse("rtmp://localhost:1935/mypath")
				require.NoError(t, err)

				nconn, err := net.Dial("tcp", u.Host)
				require.NoError(t, err)
				defer nconn.Close()

				conn, err := rtmp.NewClientConn(nconn, u, true)
				require.NoError(t, err)

				_, err = rtmp.NewWriter(conn, testFormatH264, nil)
				require.NoError(t, err)

			case "webrtc":
				u, err := url.Parse("http://localhost:8889/mypath/whip")
				require.NoError(t, err)

				c := &webrtc.WHIPClient{
					HTTPClient: hc,
					URL:        u,
				}

				_, err = c.Publish(context.Background(), medi.Formats[0], nil)
				require.NoError(t, err)
				defer func() {
					require.Error(t, c.Close())
				}()

			case "srt":
				conf := srt.DefaultConfig()
				conf.StreamId = "publish:mypath"

				conn, err := srt.Dial("srt", "localhost:8890", conf)
				require.NoError(t, err)
				defer conn.Close()

				track := &mpegts.Track{
					Codec: &mpegts.CodecH264{},
				}

				bw := bufio.NewWriter(conn)
				w := mpegts.NewWriter(bw, []*mpegts.Track{track})
				require.NoError(t, err)

				err = w.WriteH26x(track, 0, 0, true, [][]byte{{1}})
				require.NoError(t, err)

				err = bw.Flush()
				require.NoError(t, err)
			}

			var pa string
			switch ca {
			case "rtsp":
				pa = "rtspsessions"

			case "rtsps":
				pa = "rtspssessions"

			case "rtmp":
				pa = "rtmpconns"

			case "webrtc":
				pa = "webrtcsessions"

			case "srt":
				pa = "srtconns"
			}

			var out1 struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			}
			httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/"+pa+"/list", nil, &out1)

			httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/"+pa+"/kick/"+out1.Items[0].ID, nil, nil)

			var out2 struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			}
			httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/"+pa+"/list", nil, &out2)
			require.Equal(t, 0, len(out2.Items))
		})
	}
}

func TestAPIProtocolKickNotFound(t *testing.T) {
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
		"webrtc",
		"srt",
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
				"  all_others:\n"

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.Close()

			hc := &http.Client{Transport: &http.Transport{}}

			var pa string
			switch ca {
			case "rtsp":
				pa = "rtspsessions"

			case "rtsps":
				pa = "rtspssessions"

			case "rtmp":
				pa = "rtmpconns"

			case "webrtc":
				pa = "webrtcsessions"

			case "srt":
				pa = "srtconns"
			}

			func() {
				req, err := http.NewRequest(http.MethodPost, "http://localhost:9997/v3/"+pa+"/kick/"+uuid.New().String(), nil)
				require.NoError(t, err)

				res, err := hc.Do(req)
				require.NoError(t, err)
				defer res.Body.Close()

				require.Equal(t, http.StatusNotFound, res.StatusCode)

				switch ca {
				case "rtsp conns", "rtsps conns", "rtmp", "rtmps", "srt":
					checkError(t, "connection not found", res.Body)

				case "rtsp sessions", "rtsps sessions", "webrtc":
					checkError(t, "session not found", res.Body)

				case "hls":
					checkError(t, "muxer not found", res.Body)
				}
			}()
		})
	}
}
