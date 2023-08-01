//go:build enable_highlevel_tests
// +build enable_highlevel_tests

package highleveltests

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHLSServerRead(t *testing.T) {
	p, ok := newInstance("paths:\n" +
		"  all:\n")
	require.Equal(t, true, ok)
	defer p.Close()

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
	for _, result := range []string{
		"success",
		"fail",
	} {
		t.Run(result, func(t *testing.T) {
			conf := "paths:\n" +
				"  all:\n" +
				"    readUser: testreader\n" +
				"    readPass: testpass\n" +
				"    readIPs: [127.0.0.0/16]\n"

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.Close()

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

			var usr string
			if result == "success" {
				usr = "testreader"
			} else {
				usr = "testreader2"
			}

			hc := &http.Client{Transport: &http.Transport{}}

			res, err := hc.Get("http://" + usr + ":testpass@127.0.0.1:8888/teststream/index.m3u8?param=value")
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
