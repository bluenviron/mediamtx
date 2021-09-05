package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRTMPSource(t *testing.T) {
	for _, source := range []string{
		"videoaudio",
		"video",
	} {
		t.Run(source, func(t *testing.T) {
			cnt1, err := newContainer("nginx-rtmp", "rtmpserver", []string{})
			require.NoError(t, err)
			defer cnt1.close()

			cnt2, err := newContainer("ffmpeg", "source", []string{
				"-re",
				"-stream_loop", "-1",
				"-i", "empty" + source + ".mkv",
				"-c", "copy",
				"-f", "flv",
				"rtmp://" + cnt1.ip() + "/stream/test",
			})
			require.NoError(t, err)
			defer cnt2.close()

			time.Sleep(1 * time.Second)

			p, ok := newInstance("hlsDisable: yes\n" +
				"rtmpDisable: yes\n" +
				"paths:\n" +
				"  proxied:\n" +
				"    source: rtmp://localhost/stream/test\n" +
				"    sourceOnDemand: yes\n")
			require.Equal(t, true, ok)
			defer p.close()

			time.Sleep(1 * time.Second)

			cnt3, err := newContainer("ffmpeg", "dest", []string{
				"-rtsp_transport", "udp",
				"-i", "rtsp://localhost:8554/proxied",
				"-vframes", "1",
				"-f", "image2",
				"-y", "/dev/null",
			})
			require.NoError(t, err)
			defer cnt3.close()
			require.Equal(t, 0, cnt3.wait())
		})
	}
}
