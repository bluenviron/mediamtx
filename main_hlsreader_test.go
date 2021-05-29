package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClientHLSRead(t *testing.T) {
	p, ok := testProgram("")
	require.Equal(t, true, ok)
	defer p.close()

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.mkv",
		"-c", "copy",
		"-f", "rtsp",
		"rtsp://" + ownDockerIP + ":8554/test/stream",
	})
	require.NoError(t, err)
	defer cnt1.close()

	time.Sleep(1 * time.Second)

	cnt2, err := newContainer("ffmpeg", "dest", []string{
		"-i", "http://" + ownDockerIP + ":8888/test/stream/stream.m3u8",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()
	require.Equal(t, 0, cnt2.wait())
}

func TestClientHLSReadAuth(t *testing.T) {
	p, ok := testProgram(
		"paths:\n" +
			"  all:\n" +
			"    readUser: testuser\n" +
			"    readPass: testpass\n" +
			"    readIps: [172.17.0.0/16]\n")
	require.Equal(t, true, ok)
	defer p.close()

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.mkv",
		"-c", "copy",
		"-f", "rtsp",
		"rtsp://" + ownDockerIP + ":8554/teststream",
	})
	require.NoError(t, err)
	defer cnt1.close()

	time.Sleep(1 * time.Second)

	cnt2, err := newContainer("ffmpeg", "dest", []string{
		"-i", "http://testuser:testpass@" + ownDockerIP + ":8888/teststream/stream.m3u8",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()
	require.Equal(t, 0, cnt2.wait())
}
