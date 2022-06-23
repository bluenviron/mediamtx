package core

import (
	"os"
	"testing"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/pion/rtp"
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

				p, ok := newInstance("rtmpDisable: yes\n" +
					"hlsDisable: yes\n" +
					"readTimeout: 20s\n" +
					"paths:\n" +
					"  all:\n")
				require.Equal(t, true, ok)
				defer p.close()
			} else {
				proto = "rtsps"
				port = "8322"

				serverCertFpath, err := writeTempFile(serverCert)
				require.NoError(t, err)
				defer os.Remove(serverCertFpath)

				serverKeyFpath, err := writeTempFile(serverKey)
				require.NoError(t, err)
				defer os.Remove(serverKeyFpath)

				p, ok := newInstance("rtmpDisable: yes\n" +
					"hlsDisable: yes\n" +
					"readTimeout: 20s\n" +
					"protocols: [tcp]\n" +
					"encryption: \"yes\"\n" +
					"serverCert: " + serverCertFpath + "\n" +
					"serverKey: " + serverKeyFpath + "\n" +
					"paths:\n" +
					"  all:\n")
				require.Equal(t, true, ok)
				defer p.close()
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

func TestRTSPServerAuth(t *testing.T) {
	for _, ca := range []string{
		"internal",
		"external",
	} {
		t.Run(ca, func(t *testing.T) {
			var conf string
			if ca == "internal" {
				conf = "rtmpDisable: yes\n" +
					"hlsDisable: yes\n" +
					"paths:\n" +
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

			track := &gortsplib.TrackH264{
				PayloadType: 96,
				SPS:         []byte{0x01, 0x02, 0x03, 0x04},
				PPS:         []byte{0x01, 0x02, 0x03, 0x04},
			}

			source := gortsplib.Client{}

			err := source.StartPublishing(
				"rtsp://testpublisher:testpass@127.0.0.1:8554/teststream?param=value",
				gortsplib.Tracks{track})
			require.NoError(t, err)
			defer source.Close()

			if ca == "external" {
				a.close()
				var err error
				a, err = newTestHTTPAuthenticator("read")
				require.NoError(t, err)
				defer a.close()
			}

			reader := gortsplib.Client{}

			err = reader.StartReading("rtsp://testreader:testpass@127.0.0.1:8554/teststream?param=value")
			require.NoError(t, err)
			defer reader.Close()
		})
	}

	t.Run("hashed", func(t *testing.T) {
		p, ok := newInstance("rtmpDisable: yes\n" +
			"hlsDisable: yes\n" +
			"paths:\n" +
			"  all:\n" +
			"    publishUser: sha256:rl3rgi4NcZkpAEcacZnQ2VuOfJ0FxAqCRaKB/SwdZoQ=\n" +
			"    publishPass: sha256:E9JJ8stBJ7QM+nV4ZoUCeHk/gU3tPFh/5YieiJp6n2w=\n")
		require.Equal(t, true, ok)
		defer p.close()

		track := &gortsplib.TrackH264{
			PayloadType: 96,
			SPS:         []byte{0x01, 0x02, 0x03, 0x04},
			PPS:         []byte{0x01, 0x02, 0x03, 0x04},
		}

		source := gortsplib.Client{}

		err := source.StartPublishing(
			"rtsp://testuser:testpass@127.0.0.1:8554/test/stream",
			gortsplib.Tracks{track})
		require.NoError(t, err)
		defer source.Close()
	})
}

func TestRTSPServerAuthFail(t *testing.T) {
	for _, ca := range []struct {
		name string
		user string
		pass string
	}{
		{
			"wronguser",
			"test1user",
			"testpass",
		},
		{
			"wrongpass",
			"testuser",
			"test1pass",
		},
		{
			"wrongboth",
			"test1user",
			"test1pass",
		},
	} {
		t.Run("publish_"+ca.name, func(t *testing.T) {
			p, ok := newInstance("rtmpDisable: yes\n" +
				"hlsDisable: yes\n" +
				"paths:\n" +
				"  all:\n" +
				"    publishUser: testuser\n" +
				"    publishPass: testpass\n")
			require.Equal(t, true, ok)
			defer p.close()

			track := &gortsplib.TrackH264{
				PayloadType: 96,
				SPS:         []byte{0x01, 0x02, 0x03, 0x04},
				PPS:         []byte{0x01, 0x02, 0x03, 0x04},
			}

			c := gortsplib.Client{}

			err := c.StartPublishing(
				"rtsp://"+ca.user+":"+ca.pass+"@localhost:8554/test/stream",
				gortsplib.Tracks{track},
			)
			require.EqualError(t, err, "bad status code: 401 (Unauthorized)")
		})
	}

	for _, ca := range []struct {
		name string
		user string
		pass string
	}{
		{
			"wronguser",
			"test1user",
			"testpass",
		},
		{
			"wrongpass",
			"testuser",
			"test1pass",
		},
		{
			"wrongboth",
			"test1user",
			"test1pass",
		},
	} {
		t.Run("read_"+ca.name, func(t *testing.T) {
			p, ok := newInstance("rtmpDisable: yes\n" +
				"hlsDisable: yes\n" +
				"paths:\n" +
				"  all:\n" +
				"    readUser: testuser\n" +
				"    readPass: testpass\n")
			require.Equal(t, true, ok)
			defer p.close()

			c := gortsplib.Client{}

			err := c.StartReading(
				"rtsp://" + ca.user + ":" + ca.pass + "@localhost:8554/test/stream",
			)
			require.EqualError(t, err, "bad status code: 401 (Unauthorized)")
		})
	}

	t.Run("ip", func(t *testing.T) {
		p, ok := newInstance("rtmpDisable: yes\n" +
			"hlsDisable: yes\n" +
			"paths:\n" +
			"  all:\n" +
			"    publishIPs: [128.0.0.1/32]\n")
		require.Equal(t, true, ok)
		defer p.close()

		track := &gortsplib.TrackH264{
			PayloadType: 96,
			SPS:         []byte{0x01, 0x02, 0x03, 0x04},
			PPS:         []byte{0x01, 0x02, 0x03, 0x04},
		}

		c := gortsplib.Client{}

		err := c.StartPublishing(
			"rtsp://localhost:8554/test/stream",
			gortsplib.Tracks{track},
		)
		require.EqualError(t, err, "bad status code: 401 (Unauthorized)")
	})

	t.Run("external", func(t *testing.T) {
		p, ok := newInstance("externalAuthenticationURL: http://localhost:9120/auth\n" +
			"paths:\n" +
			"  all:\n")
		require.Equal(t, true, ok)
		defer p.close()

		a, err := newTestHTTPAuthenticator("publish")
		require.NoError(t, err)
		defer a.close()

		track := &gortsplib.TrackH264{
			PayloadType: 96,
			SPS:         []byte{0x01, 0x02, 0x03, 0x04},
			PPS:         []byte{0x01, 0x02, 0x03, 0x04},
		}

		c := gortsplib.Client{}

		err = c.StartPublishing(
			"rtsp://testpublisher2:testpass@localhost:8554/teststream?param=value",
			gortsplib.Tracks{track},
		)
		require.EqualError(t, err, "bad status code: 401 (Unauthorized)")
	})
}

func TestRTSPServerPublisherOverride(t *testing.T) {
	for _, ca := range []string{
		"enabled",
		"disabled",
	} {
		t.Run(ca, func(t *testing.T) {
			conf := "rtmpDisable: yes\n" +
				"protocols: [tcp]\n" +
				"paths:\n" +
				"  all:\n"

			if ca == "disabled" {
				conf += "    disablePublisherOverride: yes\n"
			}

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.close()

			track := &gortsplib.TrackH264{
				PayloadType: 96,
				SPS:         []byte{0x01, 0x02, 0x03, 0x04},
				PPS:         []byte{0x01, 0x02, 0x03, 0x04},
			}

			s1 := gortsplib.Client{}

			err := s1.StartPublishing("rtsp://localhost:8554/teststream",
				gortsplib.Tracks{track})
			require.NoError(t, err)
			defer s1.Close()

			s2 := gortsplib.Client{}

			err = s2.StartPublishing("rtsp://localhost:8554/teststream",
				gortsplib.Tracks{track})
			if ca == "enabled" {
				require.NoError(t, err)
				defer s2.Close()
			} else {
				require.Error(t, err)
			}

			frameRecv := make(chan struct{})

			c := gortsplib.Client{
				OnPacketRTP: func(ctx *gortsplib.ClientOnPacketRTPCtx) {
					if ca == "enabled" {
						require.Equal(t, []byte{0x05, 0x06, 0x07, 0x08}, ctx.Packet.Payload)
					} else {
						require.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, ctx.Packet.Payload)
					}
					close(frameRecv)
				},
			}

			err = c.StartReading("rtsp://localhost:8554/teststream")
			require.NoError(t, err)
			defer c.Close()

			err = s1.WritePacketRTP(0, &rtp.Packet{
				Header: rtp.Header{
					Version:        0x02,
					PayloadType:    97,
					SequenceNumber: 57899,
					Timestamp:      345234345,
					SSRC:           978651231,
					Marker:         true,
				},
				Payload: []byte{0x01, 0x02, 0x03, 0x04},
			}, true)
			if ca == "enabled" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			if ca == "enabled" {
				err = s2.WritePacketRTP(0, &rtp.Packet{
					Header: rtp.Header{
						Version:        0x02,
						PayloadType:    97,
						SequenceNumber: 57899,
						Timestamp:      345234345,
						SSRC:           978651231,
						Marker:         true,
					},
					Payload: []byte{0x05, 0x06, 0x07, 0x08},
				}, true)
				require.NoError(t, err)
			}

			<-frameRecv
		})
	}
}

func TestRTSPServerRedirect(t *testing.T) {
	p1, ok := newInstance("rtmpDisable: yes\n" +
		"hlsDisable: yes\n" +
		"paths:\n" +
		"  path1:\n" +
		"    source: redirect\n" +
		"    sourceRedirect: rtsp://localhost:8554/path2\n" +
		"  path2:\n")
	require.Equal(t, true, ok)
	defer p1.close()

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

func TestRTSPServerFallback(t *testing.T) {
	for _, ca := range []string{
		"absolute",
		"relative",
	} {
		t.Run(ca, func(t *testing.T) {
			val := func() string {
				if ca == "absolute" {
					return "rtsp://localhost:8554/path2"
				}
				return "/path2"
			}()

			p1, ok := newInstance("rtmpDisable: yes\n" +
				"hlsDisable: yes\n" +
				"paths:\n" +
				"  path1:\n" +
				"    fallback: " + val + "\n" +
				"  path2:\n")
			require.Equal(t, true, ok)
			defer p1.close()

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
		})
	}
}
