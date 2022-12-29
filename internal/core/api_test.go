package core

import (
	"bytes"
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

	"github.com/aler9/gortsplib/v2"
	"github.com/aler9/gortsplib/v2/pkg/codecs/mpeg4audio"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/media"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/aler9/rtsp-simple-server/internal/rtmp"
)

var testFormatH264 = &format.H264{
	PayloadTyp:        96,
	SPS:               []byte{0x01, 0x02, 0x03, 0x04},
	PPS:               []byte{0x01, 0x02, 0x03, 0x04},
	PacketizationMode: 1,
}

var testMediaH264 = &media.Media{
	Type:    media.TypeVideo,
	Formats: []format.Format{testFormatH264},
}

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
	// since the HTTP server is created and deleted multiple times,
	// we can't reuse TCP connections.
	http.DefaultTransport.(*http.Transport).DisableKeepAlives = true

	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.Close()

	var out map[string]interface{}
	err := httpRequest(http.MethodGet, "http://localhost:9997/v1/config/get", nil, &out)
	require.NoError(t, err)
	require.Equal(t, true, out["api"])
}

func TestAPIConfigSet(t *testing.T) {
	p, ok := newInstance("api: yes\n")
	require.Equal(t, true, ok)
	defer p.Close()

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
	defer p.Close()

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
	defer p.Close()

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
	defer p.Close()

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
	type pathSource struct {
		Type string `json:"type"`
	}

	type path struct {
		Source        pathSource `json:"source"`
		SourceReady   bool       `json:"sourceReady"`
		Tracks        []string   `json:"tracks"`
		BytesReceived uint64     `json:"bytesReceived"`
	}

	type pathList struct {
		Items map[string]path `json:"items"`
	}

	t.Run("rtsp session", func(t *testing.T) {
		p, ok := newInstance("api: yes\n" +
			"paths:\n" +
			"  mypath:\n")
		require.Equal(t, true, ok)
		defer p.Close()

		media0 := testMediaH264

		source := gortsplib.Client{}
		err := source.StartRecording(
			"rtsp://localhost:8554/mypath",
			media.Medias{
				media0,
				{
					Type: media.TypeAudio,
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
				},
			})
		require.NoError(t, err)
		defer source.Close()

		source.WritePacketRTP(media0, &rtp.Packet{
			Header: rtp.Header{
				Version:     2,
				PayloadType: 96,
			},
			Payload: []byte{0x01, 0x02, 0x03, 0x04},
		})

		var out pathList
		err = httpRequest(http.MethodGet, "http://localhost:9997/v1/paths/list", nil, &out)
		require.NoError(t, err)
		require.Equal(t, pathList{
			Items: map[string]path{
				"mypath": {
					Source: pathSource{
						Type: "rtspSession",
					},
					SourceReady:   true,
					Tracks:        []string{"H264", "MPEG4-audio"},
					BytesReceived: 16,
				},
			},
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

		medias := media.Medias{
			{
				Type:    media.TypeVideo,
				Formats: []format.Format{testFormatH264},
			},
			{
				Type: media.TypeAudio,
				Formats: []format.Format{&format.MPEG4Audio{
					PayloadTyp: 97,
					Config: &mpeg4audio.Config{
						Type:         2,
						SampleRate:   44100,
						ChannelCount: 2,
					},
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
				}},
			},
		}

		source := gortsplib.Client{TLSConfig: &tls.Config{InsecureSkipVerify: true}}
		err = source.StartRecording("rtsps://localhost:8322/mypath", medias)
		require.NoError(t, err)
		defer source.Close()

		var out pathList
		err = httpRequest(http.MethodGet, "http://localhost:9997/v1/paths/list", nil, &out)
		require.NoError(t, err)
		require.Equal(t, pathList{
			Items: map[string]path{
				"mypath": {
					Source: pathSource{
						Type: "rtspsSession",
					},
					SourceReady: true,
					Tracks:      []string{"H264", "MPEG4-audio"},
				},
			},
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

		var out pathList
		err := httpRequest(http.MethodGet, "http://localhost:9997/v1/paths/list", nil, &out)
		require.NoError(t, err)
		require.Equal(t, pathList{
			Items: map[string]path{
				"mypath": {
					Source: pathSource{
						Type: "rtspSource",
					},
					SourceReady: false,
					Tracks:      []string{},
				},
			},
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

		var out pathList
		err := httpRequest(http.MethodGet, "http://localhost:9997/v1/paths/list", nil, &out)
		require.NoError(t, err)
		require.Equal(t, pathList{
			Items: map[string]path{
				"mypath": {
					Source: pathSource{
						Type: "rtmpSource",
					},
					SourceReady: false,
					Tracks:      []string{},
				},
			},
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

		var out pathList
		err := httpRequest(http.MethodGet, "http://localhost:9997/v1/paths/list", nil, &out)
		require.NoError(t, err)
		require.Equal(t, pathList{
			Items: map[string]path{
				"mypath": {
					Source: pathSource{
						Type: "hlsSource",
					},
					SourceReady: false,
					Tracks:      []string{},
				},
			},
		}, out)
	})
}

func TestAPIProtocolSpecificList(t *testing.T) {
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
				"  all:\n"

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.Close()

			medi := testMediaH264

			switch ca {
			case "rtsp conns", "rtsp sessions":
				source := gortsplib.Client{}

				err := source.StartRecording("rtsp://localhost:8554/mypath", media.Medias{medi})
				require.NoError(t, err)
				defer source.Close()

			case "rtsps conns", "rtsps sessions":
				source := gortsplib.Client{
					TLSConfig: &tls.Config{InsecureSkipVerify: true},
				}

				err := source.StartRecording("rtsps://localhost:8322/mypath", media.Medias{medi})
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
				conn := rtmp.NewConn(nconn)

				err = conn.InitializeClient(u, true)
				require.NoError(t, err)

				videoTrack := &format.H264{
					PayloadTyp: 96,
					SPS: []byte{ // 1920x1080 baseline
						0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
						0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
						0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
					},
					PPS:               []byte{0x08, 0x06, 0x07, 0x08},
					PacketizationMode: 1,
				}

				err = conn.WriteTracks(videoTrack, nil)
				require.NoError(t, err)

				time.Sleep(500 * time.Millisecond)

			case "hls":
				source := gortsplib.Client{}

				err := source.StartRecording("rtsp://localhost:8554/mypath",
					media.Medias{medi})
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
			case "rtsp conns", "rtsp sessions", "rtsps conns", "rtsps sessions", "rtmp", "rtmps":
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

				if ca != "rtsp conns" && ca != "rtsps conns" {
					require.Equal(t, "publish", out.Items[firstID].State)
				}

			case "hls":
				var out struct {
					Items map[string]struct {
						Created     string `json:"created"`
						LastRequest string `json:"lastRequest"`
					} `json:"items"`
				}
				err = httpRequest(http.MethodGet, "http://localhost:9997/v1/hlsmuxers/list", nil, &out)
				require.NoError(t, err)

				var firstID string
				for k := range out.Items {
					firstID = k
				}

				s := fmt.Sprintf("^%d-", time.Now().Year())
				require.Regexp(t, s, out.Items[firstID].Created)
				require.Regexp(t, s, out.Items[firstID].LastRequest)
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
			defer p.Close()

			medi := testMediaH264

			switch ca {
			case "rtsp":
				source := gortsplib.Client{}

				err := source.StartRecording("rtsp://localhost:8554/mypath",
					media.Medias{medi})
				require.NoError(t, err)
				defer source.Close()

			case "rtsps":
				source := gortsplib.Client{
					TLSConfig: &tls.Config{InsecureSkipVerify: true},
				}

				err := source.StartRecording("rtsps://localhost:8322/mypath",
					media.Medias{medi})
				require.NoError(t, err)
				defer source.Close()

			case "rtmp":
				u, err := url.Parse("rtmp://localhost:1935/mypath")
				require.NoError(t, err)

				nconn, err := net.Dial("tcp", u.Host)
				require.NoError(t, err)
				defer nconn.Close()
				conn := rtmp.NewConn(nconn)

				err = conn.InitializeClient(u, true)
				require.NoError(t, err)

				videoTrack := &format.H264{
					PayloadTyp: 96,
					SPS: []byte{ // 1920x1080 baseline
						0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
						0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
						0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
					},
					PPS:               []byte{0x08, 0x06, 0x07, 0x08},
					PacketizationMode: 1,
				}

				err = conn.WriteTracks(videoTrack, nil)
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
