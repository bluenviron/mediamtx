package core //nolint:dupl

import (
	"crypto/tls"
	"net"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/mpeg4audio"
	"github.com/notedit/rtmp/format/flv/flvio"
	"github.com/stretchr/testify/require"

	"github.com/aler9/rtsp-simple-server/internal/rtmp"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/message"
)

func TestRTMPServerPublishRead(t *testing.T) {
	for _, ca := range []string{"plain", "tls"} {
		t.Run(ca, func(t *testing.T) {
			var port string
			if ca == "plain" {
				port = "1935"

				p, ok := newInstance("rtspDisable: yes\n" +
					"hlsDisable: yes\n" +
					"paths:\n" +
					"  all:\n")
				require.Equal(t, true, ok)
				defer p.close()
			} else {
				port = "1936"

				serverCertFpath, err := writeTempFile(serverCert)
				require.NoError(t, err)
				defer os.Remove(serverCertFpath)

				serverKeyFpath, err := writeTempFile(serverKey)
				require.NoError(t, err)
				defer os.Remove(serverKeyFpath)

				p, ok := newInstance("rtspDisable: yes\n" +
					"hlsDisable: yes\n" +
					"rtmpEncryption: \"yes\"\n" +
					"rtmpServerCert: " + serverCertFpath + "\n" +
					"rtmpServerKey: " + serverKeyFpath + "\n" +
					"paths:\n" +
					"  all:\n")
				require.Equal(t, true, ok)
				defer p.close()
			}

			u, err := url.Parse("rtmp://127.0.0.1:" + port + "/mystream")
			require.NoError(t, err)

			nconn1, err := func() (net.Conn, error) {
				if ca == "plain" {
					return net.Dial("tcp", u.Host)
				}
				return tls.Dial("tcp", u.Host, &tls.Config{InsecureSkipVerify: true})
			}()
			require.NoError(t, err)
			defer nconn1.Close()
			conn1 := rtmp.NewConn(nconn1)

			err = conn1.InitializeClient(u, true)
			require.NoError(t, err)

			videoTrack := &gortsplib.TrackH264{
				PayloadType: 96,
				SPS: []byte{ // 1920x1080 baseline
					0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
					0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
					0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
				},
				PPS: []byte{0x08, 0x06, 0x07, 0x08},
			}

			audioTrack := &gortsplib.TrackMPEG4Audio{
				PayloadType: 96,
				Config: &mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			}

			err = conn1.WriteTracks(videoTrack, audioTrack)
			require.NoError(t, err)

			nconn2, err := func() (net.Conn, error) {
				if ca == "plain" {
					return net.Dial("tcp", u.Host)
				}
				return tls.Dial("tcp", u.Host, &tls.Config{InsecureSkipVerify: true})
			}()
			require.NoError(t, err)
			defer nconn2.Close()
			conn2 := rtmp.NewConn(nconn2)

			err = conn2.InitializeClient(u, false)
			require.NoError(t, err)

			videoTrack1, audioTrack2, err := conn2.ReadTracks()
			require.NoError(t, err)
			require.Equal(t, videoTrack, videoTrack1)
			require.Equal(t, audioTrack, audioTrack2)

			err = conn1.WriteMessage(&message.MsgVideo{
				ChunkStreamID:   message.MsgVideoChunkStreamID,
				MessageStreamID: 0x1000000,
				IsKeyFrame:      true,
				H264Type:        flvio.AVC_NALU,
				Payload:         []byte{0x00, 0x00, 0x00, 0x04, 0x05, 0x02, 0x03, 0x04},
			})
			require.NoError(t, err)

			msg1, err := conn2.ReadMessage()
			require.NoError(t, err)
			require.Equal(t, &message.MsgVideo{
				ChunkStreamID:   message.MsgVideoChunkStreamID,
				MessageStreamID: 0x1000000,
				IsKeyFrame:      true,
				H264Type:        flvio.AVC_NALU,
				Payload: []byte{
					0x00, 0x00, 0x00, 0x19, 0x67, 0x42, 0xc0, 0x28,
					0xd9, 0x00, 0x78, 0x02, 0x27, 0xe5, 0x84, 0x00,
					0x00, 0x03, 0x00, 0x04, 0x00, 0x00, 0x03, 0x00,
					0xf0, 0x3c, 0x60, 0xc9, 0x20, 0x00, 0x00, 0x00,
					0x04, 0x08, 0x06, 0x07, 0x08, 0x00, 0x00, 0x00,
					0x04, 0x05, 0x02, 0x03, 0x04,
				},
			}, msg1)
		})
	}
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

			u1, err := url.Parse("rtmp://127.0.0.1:1935/teststream?user=testpublisher&pass=testpass&param=value")
			require.NoError(t, err)

			nconn1, err := net.Dial("tcp", u1.Host)
			require.NoError(t, err)
			defer nconn1.Close()
			conn1 := rtmp.NewConn(nconn1)

			err = conn1.InitializeClient(u1, true)
			require.NoError(t, err)

			videoTrack := &gortsplib.TrackH264{
				PayloadType: 96,
				SPS: []byte{
					0x67, 0x64, 0x00, 0x0c, 0xac, 0x3b, 0x50, 0xb0,
					0x4b, 0x42, 0x00, 0x00, 0x03, 0x00, 0x02, 0x00,
					0x00, 0x03, 0x00, 0x3d, 0x08,
				},
				PPS: []byte{
					0x68, 0xee, 0x3c, 0x80,
				},
			}

			err = conn1.WriteTracks(videoTrack, nil)
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			if ca == "external" {
				a.close()
				a, err = newTestHTTPAuthenticator("read")
				require.NoError(t, err)
				defer a.close()
			}

			u2, err := url.Parse("rtmp://127.0.0.1:1935/teststream?user=testreader&pass=testpass&param=value")
			require.NoError(t, err)

			nconn2, err := net.Dial("tcp", u2.Host)
			require.NoError(t, err)
			defer nconn2.Close()
			conn2 := rtmp.NewConn(nconn2)

			err = conn2.InitializeClient(u2, false)
			require.NoError(t, err)

			_, _, err = conn2.ReadTracks()
			require.NoError(t, err)
		})
	}
}

func TestRTMPServerAuthFail(t *testing.T) {
	t.Run("publish", func(t *testing.T) { //nolint:dupl
		p, ok := newInstance("rtspDisable: yes\n" +
			"hlsDisable: yes\n" +
			"paths:\n" +
			"  all:\n" +
			"    publishUser: testuser2\n" +
			"    publishPass: testpass\n")
		require.Equal(t, true, ok)
		defer p.close()

		u1, err := url.Parse("rtmp://127.0.0.1:1935/teststream?user=testuser&pass=testpass")
		require.NoError(t, err)

		nconn1, err := net.Dial("tcp", u1.Host)
		require.NoError(t, err)
		defer nconn1.Close()
		conn1 := rtmp.NewConn(nconn1)

		err = conn1.InitializeClient(u1, true)
		require.NoError(t, err)

		videoTrack := &gortsplib.TrackH264{
			PayloadType: 96,
			SPS: []byte{
				0x67, 0x64, 0x00, 0x0c, 0xac, 0x3b, 0x50, 0xb0,
				0x4b, 0x42, 0x00, 0x00, 0x03, 0x00, 0x02, 0x00,
				0x00, 0x03, 0x00, 0x3d, 0x08,
			},
			PPS: []byte{
				0x68, 0xee, 0x3c, 0x80,
			},
		}

		err = conn1.WriteTracks(videoTrack, nil)
		require.NoError(t, err)

		time.Sleep(500 * time.Millisecond)

		u2, err := url.Parse("rtmp://127.0.0.1:1935/teststream")
		require.NoError(t, err)

		nconn2, err := net.Dial("tcp", u2.Host)
		require.NoError(t, err)
		defer nconn2.Close()
		conn2 := rtmp.NewConn(nconn2)

		err = conn2.InitializeClient(u2, false)
		require.NoError(t, err)

		_, _, err = conn2.ReadTracks()
		require.EqualError(t, err, "EOF")
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

		u1, err := url.Parse("rtmp://127.0.0.1:1935/teststream?user=testuser1&pass=testpass")
		require.NoError(t, err)

		nconn1, err := net.Dial("tcp", u1.Host)
		require.NoError(t, err)
		defer nconn1.Close()
		conn1 := rtmp.NewConn(nconn1)

		err = conn1.InitializeClient(u1, true)
		require.NoError(t, err)

		videoTrack := &gortsplib.TrackH264{
			PayloadType: 96,
			SPS: []byte{
				0x67, 0x64, 0x00, 0x0c, 0xac, 0x3b, 0x50, 0xb0,
				0x4b, 0x42, 0x00, 0x00, 0x03, 0x00, 0x02, 0x00,
				0x00, 0x03, 0x00, 0x3d, 0x08,
			},
			PPS: []byte{
				0x68, 0xee, 0x3c, 0x80,
			},
		}

		err = conn1.WriteTracks(videoTrack, nil)
		require.NoError(t, err)

		time.Sleep(500 * time.Millisecond)

		u2, err := url.Parse("rtmp://127.0.0.1:1935/teststream")
		require.NoError(t, err)

		nconn2, err := net.Dial("tcp", u2.Host)
		require.NoError(t, err)
		defer nconn2.Close()
		conn2 := rtmp.NewConn(nconn2)

		err = conn2.InitializeClient(u2, false)
		require.NoError(t, err)

		_, _, err = conn2.ReadTracks()
		require.EqualError(t, err, "EOF")
	})

	t.Run("read", func(t *testing.T) { //nolint:dupl
		p, ok := newInstance("rtspDisable: yes\n" +
			"hlsDisable: yes\n" +
			"paths:\n" +
			"  all:\n" +
			"    readUser: testuser2\n" +
			"    readPass: testpass\n")
		require.Equal(t, true, ok)
		defer p.close()

		u1, err := url.Parse("rtmp://127.0.0.1:1935/teststream")
		require.NoError(t, err)

		nconn1, err := net.Dial("tcp", u1.Host)
		require.NoError(t, err)
		defer nconn1.Close()
		conn1 := rtmp.NewConn(nconn1)

		err = conn1.InitializeClient(u1, true)
		require.NoError(t, err)

		videoTrack := &gortsplib.TrackH264{
			PayloadType: 96,
			SPS: []byte{
				0x67, 0x64, 0x00, 0x0c, 0xac, 0x3b, 0x50, 0xb0,
				0x4b, 0x42, 0x00, 0x00, 0x03, 0x00, 0x02, 0x00,
				0x00, 0x03, 0x00, 0x3d, 0x08,
			},
			PPS: []byte{
				0x68, 0xee, 0x3c, 0x80,
			},
		}

		err = conn1.WriteTracks(videoTrack, nil)
		require.NoError(t, err)

		time.Sleep(500 * time.Millisecond)

		u2, err := url.Parse("rtmp://127.0.0.1:1935/teststream?user=testuser1&pass=testpass")
		require.NoError(t, err)

		nconn2, err := net.Dial("tcp", u2.Host)
		require.NoError(t, err)
		defer nconn2.Close()
		conn2 := rtmp.NewConn(nconn2)

		err = conn2.InitializeClient(u2, false)
		require.NoError(t, err)

		_, _, err = conn2.ReadTracks()
		require.EqualError(t, err, "EOF")
	})
}
