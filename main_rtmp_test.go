package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRTMPPublish(t *testing.T) {
	p, ok := testProgram("")
	require.Equal(t, true, ok)
	defer p.close()

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.ts",
		"-c", "copy",
		"-f", "flv",
		"rtmp://" + ownDockerIP + ":1935/test1/test2",
	})
	require.NoError(t, err)
	defer cnt1.close()

	time.Sleep(1 * time.Second)

	cnt2, err := newContainer("ffmpeg", "dest", []string{
		"-rtsp_transport", "udp",
		"-i", "rtsp://" + ownDockerIP + ":8554/test1/test2",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()
	require.Equal(t, 0, cnt2.wait())
}

func TestRTMPRead(t *testing.T) {
	p, ok := testProgram("")
	require.Equal(t, true, ok)
	defer p.close()

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.ts",
		"-c", "copy",
		"-f", "rtsp",
		"rtsp://" + ownDockerIP + ":8554/teststream",
	})
	require.NoError(t, err)
	defer cnt1.close()

	time.Sleep(1 * time.Second)

	cnt2, err := newContainer("ffmpeg", "dest", []string{
		"-i", "rtmp://" + ownDockerIP + ":1935/teststream",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()
	require.Equal(t, 0, cnt2.wait())
}

func TestRTMPAuth(t *testing.T) {
	t.Run("publish", func(t *testing.T) {
		p, ok := testProgram("rtspDisable: yes\n" +
			"paths:\n" +
			"  all:\n" +
			"    publishUser: testuser\n" +
			"    publishPass: testpass\n")
		require.Equal(t, true, ok)
		defer p.close()

		cnt1, err := newContainer("ffmpeg", "source", []string{
			"-re",
			"-stream_loop", "-1",
			"-i", "emptyvideo.ts",
			"-c", "copy",
			"-f", "flv",
			"rtmp://" + ownDockerIP + "/teststream?user=testuser&pass=testpass",
		})
		require.NoError(t, err)
		defer cnt1.close()

		time.Sleep(1 * time.Second)

		cnt2, err := newContainer("ffmpeg", "dest", []string{
			"-i", "rtmp://" + ownDockerIP + "/teststream",
			"-vframes", "1",
			"-f", "image2",
			"-y", "/dev/null",
		})
		require.NoError(t, err)
		defer cnt2.close()
		require.Equal(t, 0, cnt2.wait())
	})

	t.Run("read", func(t *testing.T) {
		p, ok := testProgram("rtspDisable: yes\n" +
			"paths:\n" +
			"  all:\n" +
			"    readUser: testuser\n" +
			"    readPass: testpass\n")
		require.Equal(t, true, ok)
		defer p.close()

		cnt1, err := newContainer("ffmpeg", "source", []string{
			"-re",
			"-stream_loop", "-1",
			"-i", "emptyvideo.ts",
			"-c", "copy",
			"-f", "flv",
			"rtmp://" + ownDockerIP + "/teststream",
		})
		require.NoError(t, err)
		defer cnt1.close()

		time.Sleep(1 * time.Second)

		cnt2, err := newContainer("ffmpeg", "dest", []string{
			"-i", "rtmp://" + ownDockerIP + "/teststream?user=testuser&pass=testpass",
			"-vframes", "1",
			"-f", "image2",
			"-y", "/dev/null",
		})
		require.NoError(t, err)
		defer cnt2.close()
		require.Equal(t, 0, cnt2.wait())
	})
}

func TestRTMPAuthFail(t *testing.T) {
	t.Run("publish", func(t *testing.T) {
		p, ok := testProgram("rtspDisable: yes\n" +
			"paths:\n" +
			"  all:\n" +
			"    publishUser: testuser2\n" +
			"    publishPass: testpass\n")
		require.Equal(t, true, ok)
		defer p.close()

		cnt1, err := newContainer("ffmpeg", "source", []string{
			"-re",
			"-stream_loop", "-1",
			"-i", "emptyvideo.ts",
			"-c", "copy",
			"-f", "flv",
			"rtmp://" + ownDockerIP + "/teststream?user=testuser&pass=testpass",
		})
		require.NoError(t, err)
		defer cnt1.close()

		time.Sleep(1 * time.Second)

		cnt2, err := newContainer("ffmpeg", "dest", []string{
			"-i", "rtmp://" + ownDockerIP + "/teststream",
			"-vframes", "1",
			"-f", "image2",
			"-y", "/dev/null",
		})
		require.NoError(t, err)
		defer cnt2.close()
		require.NotEqual(t, 0, cnt2.wait())
	})

	t.Run("read", func(t *testing.T) {
		p, ok := testProgram("rtspDisable: yes\n" +
			"paths:\n" +
			"  all:\n" +
			"    readUser: testuser2\n" +
			"    readPass: testpass\n")
		require.Equal(t, true, ok)
		defer p.close()

		cnt1, err := newContainer("ffmpeg", "source", []string{
			"-re",
			"-stream_loop", "-1",
			"-i", "emptyvideo.ts",
			"-c", "copy",
			"-f", "flv",
			"rtmp://" + ownDockerIP + "/teststream",
		})
		require.NoError(t, err)
		defer cnt1.close()

		time.Sleep(1 * time.Second)

		cnt2, err := newContainer("ffmpeg", "dest", []string{
			"-i", "rtmp://" + ownDockerIP + "/teststream?user=testuser&pass=testpass",
			"-vframes", "1",
			"-f", "image2",
			"-y", "/dev/null",
		})
		require.NoError(t, err)
		defer cnt2.close()
		require.NotEqual(t, 0, cnt2.wait())
	})
}
