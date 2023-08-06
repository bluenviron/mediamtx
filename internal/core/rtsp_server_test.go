package core

import (
	"os"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v3"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/bluenviron/gortsplib/v3/pkg/url"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestRTSPServerRunOnConnect(t *testing.T) {
	f, err := os.CreateTemp(os.TempDir(), "rtspss-runonconnect-")
	require.NoError(t, err)
	f.Close()
	defer os.Remove(f.Name())

	p, ok := newInstance(
		"runOnConnect: sh -c 'echo aa > " + f.Name() + "'\n" +
			"paths:\n" +
			"  all:\n")
	require.Equal(t, true, ok)
	defer p.Close()

	source := gortsplib.Client{}

	err = source.StartRecording(
		"rtsp://127.0.0.1:8554/mypath",
		media.Medias{testMediaH264})
	require.NoError(t, err)
	defer source.Close()

	time.Sleep(500 * time.Millisecond)

	byts, err := os.ReadFile(f.Name())
	require.NoError(t, err)
	require.Equal(t, "aa\n", string(byts))
}

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
					"  all:\n"

			case "internal":
				conf = "rtmp: no\n" +
					"hls: no\n" +
					"webrtc: no\n" +
					"paths:\n" +
					"  all:\n" +
					"    publishUser: testpublisher\n" +
					"    publishPass: testpass\n" +
					"    publishIPs: [127.0.0.0/16]\n" +
					"    readUser: testreader\n" +
					"    readPass: testpass\n" +
					"    readIPs: [127.0.0.0/16]\n"

			case "external":
				conf = "externalAuthenticationURL: http://localhost:9120/auth\n" +
					"paths:\n" +
					"  all:\n"
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
				media.Medias{medi})
			require.NoError(t, err)
			defer source.Close()

			if auth == "external" {
				a.close()
				a = newTestHTTPAuthenticator(t, "rtsp", "read")
				defer a.close()
			}

			reader := gortsplib.Client{}

			u, err := url.Parse("rtsp://testreader:testpass@127.0.0.1:8554/teststream?param=value")
			require.NoError(t, err)

			err = reader.Start(u.Scheme, u.Host)
			require.NoError(t, err)
			defer reader.Close()

			medias, baseURL, _, err := reader.Describe(u)
			require.NoError(t, err)

			err = reader.SetupAll(medias, baseURL)
			require.NoError(t, err)

			_, err = reader.Play(nil)
			require.NoError(t, err)
		})
	}
}

func TestRTSPServerAuthHashed(t *testing.T) {
	p, ok := newInstance(
		"rtmp: no\n" +
			"hls: no\n" +
			"webrtc: no\n" +
			"paths:\n" +
			"  all:\n" +
			"    publishUser: sha256:rl3rgi4NcZkpAEcacZnQ2VuOfJ0FxAqCRaKB/SwdZoQ=\n" +
			"    publishPass: sha256:E9JJ8stBJ7QM+nV4ZoUCeHk/gU3tPFh/5YieiJp6n2w=\n")
	require.Equal(t, true, ok)
	defer p.Close()

	medi := testMediaH264

	source := gortsplib.Client{}

	err := source.StartRecording(
		"rtsp://testuser:testpass@127.0.0.1:8554/test/stream",
		media.Medias{medi})
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
				"  all:\n" +
				"    publishUser: testuser\n" +
				"    publishPass: testpass\n")
			require.Equal(t, true, ok)
			defer p.Close()

			medi := testMediaH264

			c := gortsplib.Client{}

			err := c.StartRecording(
				"rtsp://"+ca.user+":"+ca.pass+"@localhost:8554/test/stream",
				media.Medias{medi},
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
		p, ok := newInstance("rtmp: no\n" +
			"hls: no\n" +
			"webrtc: no\n" +
			"paths:\n" +
			"  all:\n" +
			"    publishIPs: [128.0.0.1/32]\n")
		require.Equal(t, true, ok)
		defer p.Close()

		medi := testMediaH264

		c := gortsplib.Client{}

		err := c.StartRecording(
			"rtsp://localhost:8554/test/stream",
			media.Medias{medi},
		)
		require.EqualError(t, err, "bad status code: 401 (Unauthorized)")
	})

	t.Run("external", func(t *testing.T) {
		p, ok := newInstance("externalAuthenticationURL: http://localhost:9120/auth\n" +
			"paths:\n" +
			"  all:\n")
		require.Equal(t, true, ok)
		defer p.Close()

		a := newTestHTTPAuthenticator(t, "rtsp", "publish")
		defer a.close()

		medi := testMediaH264

		c := gortsplib.Client{}

		err := c.StartRecording(
			"rtsp://testpublisher2:testpass@localhost:8554/teststream?param=value",
			media.Medias{medi},
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
				"  all:\n"

			if ca == "disabled" {
				conf += "    overridePublisher: no\n"
			}

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.Close()

			medi := testMediaH264

			s1 := gortsplib.Client{}

			err := s1.StartRecording("rtsp://localhost:8554/teststream", media.Medias{medi})
			require.NoError(t, err)
			defer s1.Close()

			s2 := gortsplib.Client{}

			err = s2.StartRecording("rtsp://localhost:8554/teststream", media.Medias{medi})
			if ca == "enabled" {
				require.NoError(t, err)
				defer s2.Close()
			} else {
				require.Error(t, err)
			}

			frameRecv := make(chan struct{})

			c := gortsplib.Client{}

			u, err := url.Parse("rtsp://localhost:8554/teststream")
			require.NoError(t, err)

			err = c.Start(u.Scheme, u.Host)
			require.NoError(t, err)
			defer c.Close()

			medias, baseURL, _, err := c.Describe(u)
			require.NoError(t, err)

			err = c.SetupAll(medias, baseURL)
			require.NoError(t, err)

			c.OnPacketRTP(medias[0], medias[0].Formats[0], func(pkt *rtp.Packet) {
				if ca == "enabled" {
					require.Equal(t, []byte{0x05, 0x06, 0x07, 0x08}, pkt.Payload)
				} else {
					require.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, pkt.Payload)
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
					Payload: []byte{0x05, 0x06, 0x07, 0x08},
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
					Payload: []byte{0x01, 0x02, 0x03, 0x04},
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

			p1, ok := newInstance("rtmp: no\n" +
				"hls: no\n" +
				"webrtc: no\n" +
				"paths:\n" +
				"  path1:\n" +
				"    fallback: " + val + "\n" +
				"  path2:\n")
			require.Equal(t, true, ok)
			defer p1.Close()

			source := gortsplib.Client{}
			err := source.StartRecording("rtsp://localhost:8554/path2",
				media.Medias{testMediaH264})
			require.NoError(t, err)
			defer source.Close()

			u, err := url.Parse("rtsp://localhost:8554/path1")
			require.NoError(t, err)

			dest := gortsplib.Client{}
			err = dest.Start(u.Scheme, u.Host)
			require.NoError(t, err)
			defer dest.Close()

			medias, _, _, err := dest.Describe(u)
			require.NoError(t, err)
			require.Equal(t, 1, len(medias))
		})
	}
}
