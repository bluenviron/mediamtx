package core

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type testHTTPAuthenticator struct {
	action string

	s *http.Server
}

func newTestHTTPAuthenticator(action string) (*testHTTPAuthenticator, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:9120")
	if err != nil {
		return nil, err
	}

	ts := &testHTTPAuthenticator{
		action: action,
	}

	router := gin.New()
	router.POST("/auth", ts.onAuth)

	ts.s = &http.Server{Handler: router}
	go ts.s.Serve(ln)

	return ts, nil
}

func (ts *testHTTPAuthenticator) close() {
	ts.s.Shutdown(context.Background())
}

func (ts *testHTTPAuthenticator) onAuth(ctx *gin.Context) {
	var in struct {
		IP       string `json:"ip"`
		User     string `json:"user"`
		Password string `json:"password"`
		Path     string `json:"path"`
		Action   string `json:"action"`
		Query    string `json:"query"`
	}
	err := json.NewDecoder(ctx.Request.Body).Decode(&in)
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	var user string
	if ts.action == "publish" {
		user = "testpublisher"
	} else {
		user = "testreader"
	}

	if in.IP != "127.0.0.1" ||
		in.User != user ||
		in.Password != "testpass" ||
		in.Path != "teststream" ||
		in.Action != ts.action ||
		(in.Query != "user=testreader&pass=testpass&param=value" &&
			in.Query != "user=testpublisher&pass=testpass&param=value" &&
			in.Query != "param=value") {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
}

func TestHLSServerNotFound(t *testing.T) {
	p, ok := newInstance("")
	require.Equal(t, true, ok)
	defer p.close()

	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:8888/stream/", nil)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestHLSServerRead(t *testing.T) {
	p, ok := newInstance("paths:\n" +
		"  all:\n")
	require.Equal(t, true, ok)
	defer p.close()

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.mkv",
		"-c", "copy",
		"-f", "rtsp",
		"rtsp://127.0.0.1:8554/test/stream",
	})
	require.NoError(t, err)
	defer cnt1.close()

	time.Sleep(1 * time.Second)

	cnt2, err := newContainer("ffmpeg", "dest", []string{
		"-i", "http://127.0.0.1:8888/test/stream/index.m3u8",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()
	require.Equal(t, 0, cnt2.wait())
}

func TestHLSServerAuth(t *testing.T) {
	for _, mode := range []string{
		"internal",
		"external",
	} {
		for _, result := range []string{
			"success",
			"fail",
		} {
			t.Run(mode+"_"+result, func(t *testing.T) {
				var conf string
				if mode == "internal" {
					conf = "paths:\n" +
						"  all:\n" +
						"    readUser: testreader\n" +
						"    readPass: testpass\n" +
						"    readIPs: [127.0.0.0/16]\n"
				} else {
					conf = "externalAuthenticationURL: http://127.0.0.1:9120/auth\n" +
						"paths:\n" +
						"  all:\n"
				}

				p, ok := newInstance(conf)
				require.Equal(t, true, ok)
				defer p.close()

				var a *testHTTPAuthenticator
				if mode == "external" {
					var err error
					a, err = newTestHTTPAuthenticator("publish")
					require.NoError(t, err)
				}

				cnt1, err := newContainer("ffmpeg", "source", []string{
					"-re",
					"-stream_loop", "-1",
					"-i", "emptyvideo.mkv",
					"-c", "copy",
					"-f", "rtsp",
					"rtsp://testpublisher:testpass@127.0.0.1:8554/teststream?param=value",
				})
				require.NoError(t, err)
				defer cnt1.close()

				time.Sleep(1 * time.Second)

				if mode == "external" {
					a.close()
					var err error
					a, err = newTestHTTPAuthenticator("read")
					require.NoError(t, err)
					defer a.close()
				}

				var usr string
				if result == "success" {
					usr = "testreader"
				} else {
					usr = "testreader2"
				}

				res, err := http.Get("http://" + usr + ":testpass@127.0.0.1:8888/teststream/index.m3u8?param=value")
				require.NoError(t, err)
				defer res.Body.Close()

				if result == "success" {
					require.Equal(t, http.StatusOK, res.StatusCode)
				} else {
					require.Equal(t, http.StatusUnauthorized, res.StatusCode)
				}
			})
		}
	}
}
