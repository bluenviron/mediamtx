package core

import (
	"testing"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/url"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

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
			defer p.Close()

			var a *testHTTPAuthenticator
			if ca == "external" {
				var err error
				a, err = newTestHTTPAuthenticator("publish")
				require.NoError(t, err)
			}

			track := &gortsplib.TrackH264{
				PayloadType:       96,
				SPS:               []byte{0x01, 0x02, 0x03, 0x04},
				PPS:               []byte{0x01, 0x02, 0x03, 0x04},
				PacketizationMode: 1,
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

			u, err := url.Parse("rtsp://testreader:testpass@127.0.0.1:8554/teststream?param=value")
			require.NoError(t, err)

			err = reader.Start(u.Scheme, u.Host)
			require.NoError(t, err)
			defer reader.Close()

			tracks, baseURL, _, err := reader.Describe(u)
			require.NoError(t, err)

			err = reader.SetupAndPlay(tracks, baseURL)
			require.NoError(t, err)
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
		defer p.Close()

		track := &gortsplib.TrackH264{
			PayloadType:       96,
			SPS:               []byte{0x01, 0x02, 0x03, 0x04},
			PPS:               []byte{0x01, 0x02, 0x03, 0x04},
			PacketizationMode: 1,
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
			defer p.Close()

			track := &gortsplib.TrackH264{
				PayloadType:       96,
				SPS:               []byte{0x01, 0x02, 0x03, 0x04},
				PPS:               []byte{0x01, 0x02, 0x03, 0x04},
				PacketizationMode: 1,
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
			defer p.Close()

			c := gortsplib.Client{}

			u, err := url.Parse("rtsp://" + ca.user + ":" + ca.pass + "@localhost:8554/test/stream")
			require.NoError(t, err)

			err = c.Start(u.Scheme, u.Host)
			require.NoError(t, err)
			defer c.Close()

			_, _, _, err = c.Describe(u)
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
		defer p.Close()

		track := &gortsplib.TrackH264{
			PayloadType:       96,
			SPS:               []byte{0x01, 0x02, 0x03, 0x04},
			PPS:               []byte{0x01, 0x02, 0x03, 0x04},
			PacketizationMode: 1,
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
		defer p.Close()

		a, err := newTestHTTPAuthenticator("publish")
		require.NoError(t, err)
		defer a.close()

		track := &gortsplib.TrackH264{
			PayloadType:       96,
			SPS:               []byte{0x01, 0x02, 0x03, 0x04},
			PPS:               []byte{0x01, 0x02, 0x03, 0x04},
			PacketizationMode: 1,
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
			defer p.Close()

			track := &gortsplib.TrackH264{
				PayloadType:       96,
				SPS:               []byte{0x01, 0x02, 0x03, 0x04},
				PPS:               []byte{0x01, 0x02, 0x03, 0x04},
				PacketizationMode: 1,
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

			u, err := url.Parse("rtsp://localhost:8554/teststream")
			require.NoError(t, err)

			err = c.Start(u.Scheme, u.Host)
			require.NoError(t, err)
			defer c.Close()

			tracks, baseURL, _, err := c.Describe(u)
			require.NoError(t, err)

			err = c.SetupAndPlay(tracks, baseURL)
			require.NoError(t, err)

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
			})
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
				})
				require.NoError(t, err)
			}

			<-frameRecv
		})
	}
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
			defer p1.Close()

			source := gortsplib.Client{}
			err := source.StartPublishing("rtsp://localhost:8554/path2",
				gortsplib.Tracks{&gortsplib.TrackH264{
					PayloadType:       96,
					SPS:               []byte{0x01, 0x02, 0x03, 0x04},
					PPS:               []byte{0x01, 0x02, 0x03, 0x04},
					PacketizationMode: 1,
				}})
			require.NoError(t, err)
			defer source.Close()

			u, err := url.Parse("rtsp://localhost:8554/path1")
			require.NoError(t, err)

			dest := gortsplib.Client{}
			err = dest.Start(u.Scheme, u.Host)
			require.NoError(t, err)
			defer dest.Close()

			tracks, _, _, err := dest.Describe(u)
			require.NoError(t, err)
			require.Equal(t, 1, len(tracks))
		})
	}
}
