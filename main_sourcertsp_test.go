package main

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSourceRTSP(t *testing.T) {
	for _, source := range []string{
		"udp",
		"tcp",
		"tls",
	} {
		t.Run(source, func(t *testing.T) {
			switch source {
			case "udp", "tcp":
				p1, ok := testProgram("rtmpDisable: yes\n" +
					"hlsDisable: yes\n" +
					"rtspAddress: :8555\n" +
					"rtpAddress: :8100\n" +
					"rtcpAddress: :8101\n" +
					"paths:\n" +
					"  all:\n" +
					"    readUser: testuser\n" +
					"    readPass: testpass\n")
				require.Equal(t, true, ok)
				defer p1.close()

				cnt1, err := newContainer("ffmpeg", "source", []string{
					"-re",
					"-stream_loop", "-1",
					"-i", "emptyvideo.mkv",
					"-c", "copy",
					"-f", "rtsp",
					"-rtsp_transport", "udp",
					"rtsp://" + ownDockerIP + ":8555/teststream",
				})
				require.NoError(t, err)
				defer cnt1.close()

				p2, ok := testProgram("rtmpDisable: yes\n" +
					"hlsDisable: yes\n" +
					"paths:\n" +
					"  proxied:\n" +
					"    source: rtsp://testuser:testpass@localhost:8555/teststream\n" +
					"    sourceProtocol: " + source[len(""):] + "\n" +
					"    sourceOnDemand: yes\n")
				require.Equal(t, true, ok)
				defer p2.close()

			case "tls":
				serverCertFpath, err := writeTempFile(serverCert)
				require.NoError(t, err)
				defer os.Remove(serverCertFpath)

				serverKeyFpath, err := writeTempFile(serverKey)
				require.NoError(t, err)
				defer os.Remove(serverKeyFpath)

				p, ok := testProgram("rtmpDisable: yes\n" +
					"hlsDisable: yes\n" +
					"rtspAddress: :8555\n" +
					"rtpAddress: :8100\n" +
					"rtcpAddress: :8101\n" +
					"readTimeout: 20s\n" +
					"protocols: [tcp]\n" +
					"encryption: yes\n" +
					"serverCert: " + serverCertFpath + "\n" +
					"serverKey: " + serverKeyFpath + "\n" +
					"paths:\n" +
					"  all:\n" +
					"    readUser: testuser\n" +
					"    readPass: testpass\n")
				require.Equal(t, true, ok)
				defer p.close()

				cnt1, err := newContainer("ffmpeg", "source", []string{
					"-re",
					"-stream_loop", "-1",
					"-i", "emptyvideo.mkv",
					"-c", "copy",
					"-f", "rtsp",
					"rtsps://" + ownDockerIP + ":8555/teststream",
				})
				require.NoError(t, err)
				defer cnt1.close()

				time.Sleep(1 * time.Second)

				p2, ok := testProgram("rtmpDisable: yes\n" +
					"hlsDisable: yes\n" +
					"paths:\n" +
					"  proxied:\n" +
					"    source: rtsps://testuser:testpass@localhost:8555/teststream\n" +
					"    sourceFingerprint: 33949E05FFFB5FF3E8AA16F8213A6251B4D9363804BA53233C4DA9A46D6F2739\n" +
					"    sourceOnDemand: yes\n")
				require.Equal(t, true, ok)
				defer p2.close()
			}

			time.Sleep(1 * time.Second)

			cnt3, err := newContainer("ffmpeg", "dest", []string{
				"-rtsp_transport", "udp",
				"-i", "rtsp://" + ownDockerIP + ":8554/proxied",
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
