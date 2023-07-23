//go:build enable_highlevel_tests
// +build enable_highlevel_tests

package highleveltests

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRTSPServerPublishRead(t *testing.T) {
	for _, ca := range []struct {
		publisherSoft  string
		publisherProto string
		readerSoft     string
		readerProto    string
	}{
		{"ffmpeg", "udp", "ffmpeg", "udp"},
		{"ffmpeg", "udp", "ffmpeg", "multicast"},
		{"ffmpeg", "udp", "ffmpeg", "tcp"},
		{"ffmpeg", "udp", "gstreamer", "udp"},
		{"ffmpeg", "udp", "gstreamer", "multicast"},
		{"ffmpeg", "udp", "gstreamer", "tcp"},
		{"ffmpeg", "udp", "vlc", "udp"},
		{"ffmpeg", "udp", "vlc", "multicast"},
		{"ffmpeg", "udp", "vlc", "tcp"},
		{"ffmpeg", "tcp", "ffmpeg", "udp"},
		{"gstreamer", "udp", "ffmpeg", "udp"},
		{"gstreamer", "tcp", "ffmpeg", "udp"},
		{"ffmpeg", "tls", "ffmpeg", "tls"},
		{"ffmpeg", "tls", "gstreamer", "tls"},
		{"gstreamer", "tls", "ffmpeg", "tls"},
	} {
		t.Run(ca.publisherSoft+"_"+ca.publisherProto+"_"+
			ca.readerSoft+"_"+ca.readerProto, func(t *testing.T) {
			var proto string
			var port string
			if ca.publisherProto != "tls" {
				proto = "rtsp"
				port = "8554"

				p, ok := newInstance("rtmp: no\n" +
					"hls: no\n" +
					"webrtc: no\n" +
					"readTimeout: 20s\n" +
					"paths:\n" +
					"  all:\n")
				require.Equal(t, true, ok)
				defer p.Close()
			} else {
				proto = "rtsps"
				port = "8322"

				serverCertFpath, err := writeTempFile(serverCert)
				require.NoError(t, err)
				defer os.Remove(serverCertFpath)

				serverKeyFpath, err := writeTempFile(serverKey)
				require.NoError(t, err)
				defer os.Remove(serverKeyFpath)

				p, ok := newInstance("rtmp: no\n" +
					"hls: no\n" +
					"webrtc: no\n" +
					"readTimeout: 20s\n" +
					"protocols: [tcp]\n" +
					"encryption: \"yes\"\n" +
					"serverCert: " + serverCertFpath + "\n" +
					"serverKey: " + serverKeyFpath + "\n" +
					"paths:\n" +
					"  all:\n")
				require.Equal(t, true, ok)
				defer p.Close()
			}

			switch ca.publisherSoft {
			case "ffmpeg":
				ps := func() string {
					switch ca.publisherProto {
					case "udp", "tcp":
						return ca.publisherProto

					default: // tls
						return "tcp"
					}
				}()

				cnt1, err := newContainer("ffmpeg", "source", []string{
					"-re",
					"-stream_loop", "-1",
					"-i", "emptyvideo.mkv",
					"-c", "copy",
					"-f", "rtsp",
					"-rtsp_transport",
					ps,
					proto + "://localhost:" + port + "/teststream",
				})
				require.NoError(t, err)
				defer cnt1.close()

				time.Sleep(1 * time.Second)

			case "gstreamer":
				ps := func() string {
					switch ca.publisherProto {
					case "udp", "tcp":
						return ca.publisherProto

					default: // tls
						return "tcp"
					}
				}()

				cnt1, err := newContainer("gstreamer", "source", []string{
					"filesrc location=emptyvideo.mkv ! matroskademux ! video/x-h264 ! rtspclientsink " +
						"location=" + proto + "://localhost:" + port + "/teststream " +
						"protocols=" + ps + " tls-validation-flags=0 latency=0 timeout=0 rtx-time=0",
				})
				require.NoError(t, err)
				defer cnt1.close()

				time.Sleep(1 * time.Second)
			}

			time.Sleep(1 * time.Second)

			switch ca.readerSoft {
			case "ffmpeg":
				ps := func() string {
					switch ca.readerProto {
					case "udp", "tcp":
						return ca.publisherProto

					case "multicast":
						return "udp_multicast"

					default: // tls
						return "tcp"
					}
				}()

				cnt2, err := newContainer("ffmpeg", "dest", []string{
					"-rtsp_transport", ps,
					"-i", proto + "://localhost:" + port + "/teststream",
					"-vframes", "1",
					"-f", "image2",
					"-y", "/dev/null",
				})
				require.NoError(t, err)
				defer cnt2.close()
				require.Equal(t, 0, cnt2.wait())

			case "gstreamer":
				ps := func() string {
					switch ca.readerProto {
					case "udp", "tcp":
						return ca.publisherProto

					case "multicast":
						return "udp-mcast"

					default: // tls
						return "tcp"
					}
				}()

				cnt2, err := newContainer("gstreamer", "read", []string{
					"rtspsrc location=" + proto + "://127.0.0.1:" + port + "/teststream " +
						"protocols=" + ps + " " +
						"tls-validation-flags=0 latency=0 " +
						"! application/x-rtp,media=video ! decodebin ! exitafterframe ! fakesink",
				})
				require.NoError(t, err)
				defer cnt2.close()
				require.Equal(t, 0, cnt2.wait())

			case "vlc":
				args := []string{}
				if ca.readerProto == "tcp" {
					args = append(args, "--rtsp-tcp")
				}

				ur := proto + "://localhost:" + port + "/teststream"
				if ca.readerProto == "multicast" {
					ur += "?vlcmulticast"
				}

				args = append(args, ur)
				cnt2, err := newContainer("vlc", "dest", args)
				require.NoError(t, err)
				defer cnt2.close()
				require.Equal(t, 0, cnt2.wait())
			}
		})
	}
}

func TestRTSPServerRedirect(t *testing.T) {
	p1, ok := newInstance("rtmp: no\n" +
		"hls: no\n" +
		"webrtc: no\n" +
		"paths:\n" +
		"  path1:\n" +
		"    source: redirect\n" +
		"    sourceRedirect: rtsp://localhost:8554/path2\n" +
		"  path2:\n")
	require.Equal(t, true, ok)
	defer p1.Close()

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.mkv",
		"-c", "copy",
		"-f", "rtsp",
		"-rtsp_transport", "udp",
		"rtsp://localhost:8554/path2",
	})
	require.NoError(t, err)
	defer cnt1.close()

	time.Sleep(1 * time.Second)

	cnt2, err := newContainer("ffmpeg", "dest", []string{
		"-rtsp_transport", "udp",
		"-i", "rtsp://localhost:8554/path1",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()
	require.Equal(t, 0, cnt2.wait())
}
