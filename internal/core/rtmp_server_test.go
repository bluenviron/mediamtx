package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRTMPServerPublish(t *testing.T) {
	for _, source := range []string{
		"videoaudio",
		"video",
	} {
		t.Run(source, func(t *testing.T) {
			p, ok := newInstance("hlsDisable: yes\n")
			require.Equal(t, true, ok)
			defer p.close()

			cnt1, err := newContainer("ffmpeg", "source", []string{
				"-re",
				"-stream_loop", "-1",
				"-i", "empty" + source + ".mkv",
				"-c", "copy",
				"-f", "flv",
				"rtmp://localhost:1935/test1/test2",
			})
			require.NoError(t, err)
			defer cnt1.close()

			time.Sleep(1 * time.Second)

			cnt2, err := newContainer("ffmpeg", "dest", []string{
				"-rtsp_transport", "udp",
				"-i", "rtsp://localhost:8554/test1/test2",
				"-vframes", "1",
				"-f", "image2",
				"-y", "/dev/null",
			})
			require.NoError(t, err)
			defer cnt2.close()
			require.Equal(t, 0, cnt2.wait())
		})
	}
}

func TestRTMPServerRead(t *testing.T) {
	p, ok := newInstance("hlsDisable: yes\n")
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
		"-i", "rtmp://localhost:1935/teststream",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()
	require.Equal(t, 0, cnt2.wait())
}

func TestRTMPServerAuth(t *testing.T) {
	t.Run("publish", func(t *testing.T) {
		p, ok := newInstance("rtspDisable: yes\n" +
			"hlsDisable: yes\n" +
			"paths:\n" +
			"  all:\n" +
			"    publishUser: testuser\n" +
			"    publishPass: testpass\n" +
			"    readIPs: [127.0.0.0/16]\n")
		require.Equal(t, true, ok)
		defer p.close()

		cnt1, err := newContainer("ffmpeg", "source", []string{
			"-re",
			"-stream_loop", "-1",
			"-i", "emptyvideo.mkv",
			"-c", "copy",
			"-f", "flv",
			"rtmp://localhost/teststream?user=testuser&pass=testpass",
		})
		require.NoError(t, err)
		defer cnt1.close()

		time.Sleep(1 * time.Second)

		cnt2, err := newContainer("ffmpeg", "dest", []string{
			"-i", "rtmp://127.0.0.1/teststream",
			"-vframes", "1",
			"-f", "image2",
			"-y", "/dev/null",
		})
		require.NoError(t, err)
		defer cnt2.close()
		require.Equal(t, 0, cnt2.wait())
	})

	t.Run("read", func(t *testing.T) {
		p, ok := newInstance("rtspDisable: yes\n" +
			"hlsDisable: yes\n" +
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
			"-f", "flv",
			"rtmp://localhost/teststream",
		})
		require.NoError(t, err)
		defer cnt1.close()

		time.Sleep(1 * time.Second)

		cnt2, err := newContainer("ffmpeg", "dest", []string{
			"-i", "rtmp://127.0.0.1/teststream?user=testuser&pass=testpass",
			"-vframes", "1",
			"-f", "image2",
			"-y", "/dev/null",
		})
		require.NoError(t, err)
		defer cnt2.close()
		require.Equal(t, 0, cnt2.wait())
	})
}

func TestRTMPServerAuthFail(t *testing.T) {
	t.Run("publish", func(t *testing.T) {
		p, ok := newInstance("rtspDisable: yes\n" +
			"hlsDisable: yes\n" +
			"paths:\n" +
			"  all:\n" +
			"    publishUser: testuser2\n" +
			"    publishPass: testpass\n")
		require.Equal(t, true, ok)
		defer p.close()

		cnt1, err := newContainer("ffmpeg", "source", []string{
			"-re",
			"-stream_loop", "-1",
			"-i", "emptyvideo.mkv",
			"-c", "copy",
			"-f", "flv",
			"rtmp://localhost/teststream?user=testuser&pass=testpass",
		})
		require.NoError(t, err)
		defer cnt1.close()

		time.Sleep(1 * time.Second)

		cnt2, err := newContainer("ffmpeg", "dest", []string{
			"-i", "rtmp://localhost/teststream",
			"-vframes", "1",
			"-f", "image2",
			"-y", "/dev/null",
		})
		require.NoError(t, err)
		defer cnt2.close()
		require.NotEqual(t, 0, cnt2.wait())
	})

	t.Run("read", func(t *testing.T) {
		p, ok := newInstance("rtspDisable: yes\n" +
			"hlsDisable: yes\n" +
			"paths:\n" +
			"  all:\n" +
			"    readUser: testuser2\n" +
			"    readPass: testpass\n")
		require.Equal(t, true, ok)
		defer p.close()

		cnt1, err := newContainer("ffmpeg", "source", []string{
			"-re",
			"-stream_loop", "-1",
			"-i", "emptyvideo.mkv",
			"-c", "copy",
			"-f", "flv",
			"rtmp://localhost/teststream",
		})
		require.NoError(t, err)
		defer cnt1.close()

		time.Sleep(1 * time.Second)

		cnt2, err := newContainer("ffmpeg", "dest", []string{
			"-i", "rtmp://localhost/teststream?user=testuser&pass=testpass",
			"-vframes", "1",
			"-f", "image2",
			"-y", "/dev/null",
		})
		require.NoError(t, err)
		defer cnt2.close()
		require.NotEqual(t, 0, cnt2.wait())
	})
}
