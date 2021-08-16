package core

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHLSServerNotFound(t *testing.T) {
	p, ok := newInstance("")
	require.Equal(t, true, ok)
	defer p.close()

	req, err := http.NewRequest(http.MethodGet, "http://localhost:8888/stream/", nil)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestHLSServerRead(t *testing.T) {
	p, ok := newInstance("")
	require.Equal(t, true, ok)
	defer p.close()

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.mkv",
		"-c", "copy",
		"-f", "rtsp",
		"rtsp://localhost:8554/test/stream",
	})
	require.NoError(t, err)
	defer cnt1.close()

	time.Sleep(1 * time.Second)

	cnt2, err := newContainer("ffmpeg", "dest", []string{
		"-i", "http://localhost:8888/test/stream/index.m3u8",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()
	require.Equal(t, 0, cnt2.wait())
}

func TestHLSServerReadAuth(t *testing.T) {
	p, ok := newInstance(
		"paths:\n" +
			"  all:\n" +
			"    readUser: testuser\n" +
			"    readPass: testpass\n" +
			"    readIPs: [127.0.0.0/16]\n")
	require.Equal(t, true, ok)
	defer p.close()

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.mkv",
		"-c", "copy",
		"-f", "rtsp",
		"rtsp://localhost:8554/teststream",
	})
	require.NoError(t, err)
	defer cnt1.close()

	time.Sleep(1 * time.Second)

	cnt2, err := newContainer("ffmpeg", "dest", []string{
		"-i", "http://testuser:testpass@127.0.0.1:8888/teststream/index.m3u8",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()
	require.Equal(t, 0, cnt2.wait())
}
