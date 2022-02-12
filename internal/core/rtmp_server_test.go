package core

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aler9/rtsp-simple-server/internal/rtmp"
)

func TestRTMPServerPublish(t *testing.T) {
	for _, source := range []string{
		"videoaudio",
		"video",
	} {
		t.Run(source, func(t *testing.T) {
			p, ok := newInstance("hlsDisable: yes\n" +
				"paths:\n" +
				"  all:\n")
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
	p, ok := newInstance("hlsDisable: yes\n" +
		"paths:\n" +
		"  all:\n")
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
	for _, ca := range []string{
		"internal",
		"external",
	} {
		t.Run(ca, func(t *testing.T) {
			var conf string
			if ca == "internal" {
				conf = "paths:\n" +
					"  all:\n" +
					"    publishUser: testpublisher\n" +
					"    publishPass: testpass\n" +
					"    publishIPs: [127.0.0.0/16]\n" +
					"    readUser: testreader\n" +
					"    readPass: testpass\n" +
					"    readIPs: [127.0.0.0/16]\n"
			} else {
				conf = "externalAuthenticationURL: http://localhost:9120/auth\n" +
					"paths:\n" +
					"  all:\n"
			}

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.close()

			var a *testHTTPAuthenticator
			if ca == "external" {
				var err error
				a, err = newTestHTTPAuthenticator("publish")
				require.NoError(t, err)
			}

			cnt1, err := newContainer("ffmpeg", "source", []string{
				"-re",
				"-stream_loop", "-1",
				"-i", "emptyvideo.mkv",
				"-c", "copy",
				"-f", "flv",
				"rtmp://127.0.0.1/teststream?user=testpublisher&pass=testpass&param=value",
			})
			require.NoError(t, err)
			defer cnt1.close()

			time.Sleep(1 * time.Second)

			if ca == "external" {
				a.close()
				a, err = newTestHTTPAuthenticator("read")
				require.NoError(t, err)
				defer a.close()
			}

			conn, err := rtmp.DialContext(context.Background(),
				"rtmp://127.0.0.1/teststream?user=testreader&pass=testpass&param=value")
			require.NoError(t, err)
			defer conn.Close()

			err = conn.ClientHandshake()
			require.NoError(t, err)

			_, _, err = conn.ReadTracks()
			require.NoError(t, err)
		})
	}
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
		require.NotEqual(t, 0, cnt1.wait())
	})

	t.Run("publish_external", func(t *testing.T) {
		p, ok := newInstance("externalAuthenticationURL: http://localhost:9120/auth\n" +
			"paths:\n" +
			"  all:\n")
		require.Equal(t, true, ok)
		defer p.close()

		a, err := newTestHTTPAuthenticator("publish")
		require.NoError(t, err)
		defer a.close()

		cnt1, err := newContainer("ffmpeg", "source", []string{
			"-re",
			"-stream_loop", "-1",
			"-i", "emptyvideo.mkv",
			"-c", "copy",
			"-f", "flv",
			"rtmp://localhost/teststream?user=testuser2&pass=testpass&param=value",
		})
		require.NoError(t, err)
		defer cnt1.close()
		require.NotEqual(t, 0, cnt1.wait())
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

		conn, err := rtmp.DialContext(context.Background(), "rtmp://127.0.0.1/teststream?user=testuser&pass=testpass")
		require.NoError(t, err)
		defer conn.Close()

		err = conn.ClientHandshake()
		require.Equal(t, err, io.EOF)
	})
}
