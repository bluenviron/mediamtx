package core

import (
	"testing"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestRTSPServer(t *testing.T) {
	for _, auth := range []string{
		"none",
		"internal",
		"external",
	} {
		t.Run("auth_"+auth, func(t *testing.T) {
			var conf string

			switch auth {
			case "none":
				conf = "paths:\n" +
					"  all_others:\n"

			case "internal":
				conf = "rtmp: no\n" +
					"hls: no\n" +
					"webrtc: no\n" +
					"paths:\n" +
					"  all_others:\n" +
					"    publishUser: testpublisher\n" +
					"    publishPass: testpass\n" +
					"    publishIPs: [127.0.0.0/16]\n" +
					"    readUser: testreader\n" +
					"    readPass: testpass\n" +
					"    readIPs: [127.0.0.0/16]\n"

			case "external":
				conf = "externalAuthenticationURL: http://localhost:9120/auth\n" +
					"paths:\n" +
					"  all_others:\n"
			}

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.Close()

			var a *testHTTPAuthenticator
			if auth == "external" {
				a = newTestHTTPAuthenticator(t, "rtsp", "publish")
			}

			medi := testMediaH264

			source := gortsplib.Client{}

			err := source.StartRecording(
				"rtsp://testpublisher:testpass@127.0.0.1:8554/teststream?param=value",
				&description.Session{Medias: []*description.Media{medi}})
			require.NoError(t, err)
			defer source.Close()

			if auth == "external" {
				a.close()
				a = newTestHTTPAuthenticator(t, "rtsp", "read")
				defer a.close()
			}

			reader := gortsplib.Client{}

			u, err := base.ParseURL("rtsp://testreader:testpass@127.0.0.1:8554/teststream?param=value")
			require.NoError(t, err)

			err = reader.Start(u.Scheme, u.Host)
			require.NoError(t, err)
			defer reader.Close()

			desc, _, err := reader.Describe(u)
			require.NoError(t, err)

			err = reader.SetupAll(desc.BaseURL, desc.Medias)
			require.NoError(t, err)

			_, err = reader.Play(nil)
			require.NoError(t, err)
		})
	}
}

func TestRTSPServerAuthHashedSHA256(t *testing.T) {
	p, ok := newInstance(
		"rtmp: no\n" +
			"hls: no\n" +
			"webrtc: no\n" +
			"paths:\n" +
			"  all_others:\n" +
			"    publishUser: sha256:rl3rgi4NcZkpAEcacZnQ2VuOfJ0FxAqCRaKB/SwdZoQ=\n" +
			"    publishPass: sha256:E9JJ8stBJ7QM+nV4ZoUCeHk/gU3tPFh/5YieiJp6n2w=\n")
	require.Equal(t, true, ok)
	defer p.Close()

	medi := testMediaH264

	source := gortsplib.Client{}

	err := source.StartRecording(
		"rtsp://testuser:testpass@127.0.0.1:8554/test/stream",
		&description.Session{Medias: []*description.Media{medi}})
	require.NoError(t, err)
	defer source.Close()
}

func TestRTSPServerAuthHashedArgon2(t *testing.T) {
	p, ok := newInstance(
		"rtmp: no\n" +
			"hls: no\n" +
			"webrtc: no\n" +
			"paths:\n" +
			"  all_others:\n" +
			"    publishUser: argon2:$argon2id$v=19$m=4096,t=3,p=1$MTIzNDU2Nzg$Ux/LWeTgJQPyfMMJo1myR64+o8rALHoPmlE1i/TR+58\n" +
			"    publishPass: argon2:$argon2i$v=19$m=4096,t=3,p=1$MTIzNDU2Nzg$/mrZ42TiTv1mcPnpMUera5oi0SFYbbyueAbdx5sUvWo\n")
	require.Equal(t, true, ok)
	defer p.Close()

	medi := testMediaH264

	source := gortsplib.Client{}

	err := source.StartRecording(
		"rtsp://testuser:testpass@127.0.0.1:8554/test/stream",
		&description.Session{Medias: []*description.Media{medi}})
	require.NoError(t, err)
	defer source.Close()
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
			p, ok := newInstance("rtmp: no\n" +
				"hls: no\n" +
				"webrtc: no\n" +
				"paths:\n" +
				"  all_others:\n" +
				"    publishUser: testuser\n" +
				"    publishPass: testpass\n")
			require.Equal(t, true, ok)
			defer p.Close()

			medi := testMediaH264

			c := gortsplib.Client{}

			err := c.StartRecording(
				"rtsp://"+ca.user+":"+ca.pass+"@localhost:8554/test/stream",
				&description.Session{Medias: []*description.Media{medi}},
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
			p, ok := newInstance("rtmp: no\n" +
				"hls: no\n" +
				"webrtc: no\n" +
				"paths:\n" +
				"  all_others:\n" +
				"    readUser: testuser\n" +
				"    readPass: testpass\n")
			require.Equal(t, true, ok)
			defer p.Close()

			c := gortsplib.Client{}

			u, err := base.ParseURL("rtsp://" + ca.user + ":" + ca.pass + "@localhost:8554/test/stream")
			require.NoError(t, err)

			err = c.Start(u.Scheme, u.Host)
			require.NoError(t, err)
			defer c.Close()

			_, _, err = c.Describe(u)
			require.EqualError(t, err, "bad status code: 401 (Unauthorized)")
		})
	}

	t.Run("ip", func(t *testing.T) {
		p, ok := newInstance("rtmp: no\n" +
			"hls: no\n" +
			"webrtc: no\n" +
			"paths:\n" +
			"  all_others:\n" +
			"    publishIPs: [128.0.0.1/32]\n")
		require.Equal(t, true, ok)
		defer p.Close()

		medi := testMediaH264

		c := gortsplib.Client{}

		err := c.StartRecording(
			"rtsp://localhost:8554/test/stream",
			&description.Session{Medias: []*description.Media{medi}},
		)
		require.EqualError(t, err, "bad status code: 401 (Unauthorized)")
	})

	t.Run("external", func(t *testing.T) {
		p, ok := newInstance("externalAuthenticationURL: http://localhost:9120/auth\n" +
			"paths:\n" +
			"  all_others:\n")
		require.Equal(t, true, ok)
		defer p.Close()

		a := newTestHTTPAuthenticator(t, "rtsp", "publish")
		defer a.close()

		medi := testMediaH264

		c := gortsplib.Client{}

		err := c.StartRecording(
			"rtsp://testpublisher2:testpass@localhost:8554/teststream?param=value",
			&description.Session{Medias: []*description.Media{medi}},
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
			conf := "rtmp: no\n" +
				"paths:\n" +
				"  all_others:\n"

			if ca == "disabled" {
				conf += "    overridePublisher: no\n"
			}

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.Close()

			medi := testMediaH264

			s1 := gortsplib.Client{}

			err := s1.StartRecording("rtsp://localhost:8554/teststream",
				&description.Session{Medias: []*description.Media{medi}})
			require.NoError(t, err)
			defer s1.Close()

			s2 := gortsplib.Client{}

			err = s2.StartRecording("rtsp://localhost:8554/teststream",
				&description.Session{Medias: []*description.Media{medi}})
			if ca == "enabled" {
				require.NoError(t, err)
				defer s2.Close()
			} else {
				require.Error(t, err)
			}

			frameRecv := make(chan struct{})

			c := gortsplib.Client{}

			u, err := base.ParseURL("rtsp://localhost:8554/teststream")
			require.NoError(t, err)

			err = c.Start(u.Scheme, u.Host)
			require.NoError(t, err)
			defer c.Close()

			desc, _, err := c.Describe(u)
			require.NoError(t, err)

			err = c.SetupAll(desc.BaseURL, desc.Medias)
			require.NoError(t, err)

			c.OnPacketRTP(desc.Medias[0], desc.Medias[0].Formats[0], func(pkt *rtp.Packet) {
				if ca == "enabled" {
					require.Equal(t, []byte{5, 15, 16, 17, 18}, pkt.Payload)
				} else {
					require.Equal(t, []byte{5, 11, 12, 13, 14}, pkt.Payload)
				}
				close(frameRecv)
			})

			_, err = c.Play(nil)
			require.NoError(t, err)

			if ca == "enabled" {
				err := s1.Wait()
				require.EqualError(t, err, "EOF")

				err = s2.WritePacketRTP(medi, &rtp.Packet{
					Header: rtp.Header{
						Version:        0x02,
						PayloadType:    96,
						SequenceNumber: 57899,
						Timestamp:      345234345,
						SSRC:           978651231,
						Marker:         true,
					},
					Payload: []byte{5, 15, 16, 17, 18},
				})
				require.NoError(t, err)
			} else {
				err = s1.WritePacketRTP(medi, &rtp.Packet{
					Header: rtp.Header{
						Version:        0x02,
						PayloadType:    96,
						SequenceNumber: 57899,
						Timestamp:      345234345,
						SSRC:           978651231,
						Marker:         true,
					},
					Payload: []byte{5, 11, 12, 13, 14},
				})
				require.NoError(t, err)
			}

			<-frameRecv
		})
	}
}
