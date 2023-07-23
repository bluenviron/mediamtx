package core //nolint:dupl

import (
	"crypto/tls"
	"net"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/rtmp"
)

func TestRTMPServerRunOnConnect(t *testing.T) {
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

	u, err := url.Parse("rtmp://127.0.0.1:1935/teststream")
	require.NoError(t, err)

	nconn, err := net.Dial("tcp", u.Host)
	require.NoError(t, err)
	defer nconn.Close()

	_, err = rtmp.NewClientConn(nconn, u, true)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	byts, err := os.ReadFile(f.Name())
	require.NoError(t, err)
	require.Equal(t, "aa\n", string(byts))
}

func TestRTMPServer(t *testing.T) {
	for _, encrypt := range []string{
		"plain",
		"tls",
	} {
		for _, auth := range []string{
			"none",
			"internal",
			"external",
		} {
			t.Run("encrypt_"+encrypt+"_auth_"+auth, func(t *testing.T) {
				var port string
				var conf string

				if encrypt == "plain" {
					port = "1935"

					conf = "rtsp: no\n" +
						"hls: no\n"
				} else {
					port = "1936"

					serverCertFpath, err := writeTempFile(serverCert)
					require.NoError(t, err)
					defer os.Remove(serverCertFpath)

					serverKeyFpath, err := writeTempFile(serverKey)
					require.NoError(t, err)
					defer os.Remove(serverKeyFpath)

					conf = "rtsp: no\n" +
						"hls: no\n" +
						"webrtc: no\n" +
						"rtmpEncryption: \"yes\"\n" +
						"rtmpServerCert: " + serverCertFpath + "\n" +
						"rtmpServerKey: " + serverKeyFpath + "\n"
				}

				switch auth {
				case "none":
					conf += "paths:\n" +
						"  all:\n"

				case "internal":
					conf += "paths:\n" +
						"  all:\n" +
						"    publishUser: testpublisher\n" +
						"    publishPass: testpass\n" +
						"    publishIPs: [127.0.0.0/16]\n" +
						"    readUser: testreader\n" +
						"    readPass: testpass\n" +
						"    readIPs: [127.0.0.0/16]\n"

				case "external":
					conf += "externalAuthenticationURL: http://localhost:9120/auth\n" +
						"paths:\n" +
						"  all:\n"
				}

				p, ok := newInstance(conf)
				require.Equal(t, true, ok)
				defer p.Close()

				var a *testHTTPAuthenticator
				if auth == "external" {
					a = newTestHTTPAuthenticator(t, "rtmp", "publish")
				}

				u1, err := url.Parse("rtmp://127.0.0.1:" + port + "/teststream?user=testpublisher&pass=testpass&param=value")
				require.NoError(t, err)

				nconn1, err := func() (net.Conn, error) {
					if encrypt == "plain" {
						return net.Dial("tcp", u1.Host)
					}
					return tls.Dial("tcp", u1.Host, &tls.Config{InsecureSkipVerify: true})
				}()
				require.NoError(t, err)
				defer nconn1.Close()

				conn1, err := rtmp.NewClientConn(nconn1, u1, true)
				require.NoError(t, err)

				videoTrack := &formats.H264{
					PayloadTyp: 96,
					SPS: []byte{ // 1920x1080 baseline
						0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
						0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
						0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
					},
					PPS:               []byte{0x08, 0x06, 0x07, 0x08},
					PacketizationMode: 1,
				}

				audioTrack := &formats.MPEG4Audio{
					PayloadTyp: 96,
					Config: &mpeg4audio.Config{
						Type:         2,
						SampleRate:   44100,
						ChannelCount: 2,
					},
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
				}

				w, err := rtmp.NewWriter(conn1, videoTrack, audioTrack)
				require.NoError(t, err)

				time.Sleep(500 * time.Millisecond)

				if auth == "external" {
					a.close()
					a = newTestHTTPAuthenticator(t, "rtmp", "read")
					defer a.close()
				}

				u2, err := url.Parse("rtmp://127.0.0.1:" + port + "/teststream?user=testreader&pass=testpass&param=value")
				require.NoError(t, err)

				nconn2, err := func() (net.Conn, error) {
					if encrypt == "plain" {
						return net.Dial("tcp", u2.Host)
					}
					return tls.Dial("tcp", u2.Host, &tls.Config{InsecureSkipVerify: true})
				}()
				require.NoError(t, err)
				defer nconn2.Close()

				conn2, err := rtmp.NewClientConn(nconn2, u2, false)
				require.NoError(t, err)

				r, err := rtmp.NewReader(conn2)
				require.NoError(t, err)
				videoTrack1, audioTrack2 := r.Tracks()
				require.Equal(t, videoTrack, videoTrack1)
				require.Equal(t, audioTrack, audioTrack2)

				err = w.WriteH264(0, 0, true, [][]byte{
					{0x05, 0x02, 0x03, 0x04}, // IDR 1
					{0x05, 0x02, 0x03, 0x04}, // IDR 2
				})
				require.NoError(t, err)

				r.OnDataH264(func(pts time.Duration, au [][]byte) {
					require.Equal(t, [][]byte{
						{ // SPS
							0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
							0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
							0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9,
							0x20,
						},
						{ // PPS
							0x08, 0x06, 0x07, 0x08,
						},
						{ // IDR 1
							0x05, 0x02, 0x03, 0x04,
						},
						{ // IDR 2
							0x05, 0x02, 0x03, 0x04,
						},
					}, au)
				})

				err = r.Read()
				require.NoError(t, err)
			})
		}
	}
}

func TestRTMPServerAuthFail(t *testing.T) {
	t.Run("publish", func(t *testing.T) { //nolint:dupl
		p, ok := newInstance("rtsp: no\n" +
			"hls: no\n" +
			"webrtc: no\n" +
			"paths:\n" +
			"  all:\n" +
			"    publishUser: testuser2\n" +
			"    publishPass: testpass\n")
		require.Equal(t, true, ok)
		defer p.Close()

		u1, err := url.Parse("rtmp://127.0.0.1:1935/teststream?user=testuser&pass=testpass")
		require.NoError(t, err)

		nconn1, err := net.Dial("tcp", u1.Host)
		require.NoError(t, err)
		defer nconn1.Close()

		conn1, err := rtmp.NewClientConn(nconn1, u1, true)
		require.NoError(t, err)

		videoTrack := &formats.H264{
			PayloadTyp: 96,
			SPS: []byte{
				0x67, 0x64, 0x00, 0x0c, 0xac, 0x3b, 0x50, 0xb0,
				0x4b, 0x42, 0x00, 0x00, 0x03, 0x00, 0x02, 0x00,
				0x00, 0x03, 0x00, 0x3d, 0x08,
			},
			PPS: []byte{
				0x68, 0xee, 0x3c, 0x80,
			},
			PacketizationMode: 1,
		}

		_, err = rtmp.NewWriter(conn1, videoTrack, nil)
		require.NoError(t, err)

		time.Sleep(500 * time.Millisecond)

		u2, err := url.Parse("rtmp://127.0.0.1:1935/teststream")
		require.NoError(t, err)

		nconn2, err := net.Dial("tcp", u2.Host)
		require.NoError(t, err)
		defer nconn2.Close()

		conn2, err := rtmp.NewClientConn(nconn2, u2, false)
		require.NoError(t, err)

		_, err = rtmp.NewReader(conn2)
		require.EqualError(t, err, "EOF")
	})

	t.Run("publish_external", func(t *testing.T) {
		p, ok := newInstance("externalAuthenticationURL: http://localhost:9120/auth\n" +
			"paths:\n" +
			"  all:\n")
		require.Equal(t, true, ok)
		defer p.Close()

		a := newTestHTTPAuthenticator(t, "rtmp", "publish")
		defer a.close()

		u1, err := url.Parse("rtmp://127.0.0.1:1935/teststream?user=testuser1&pass=testpass")
		require.NoError(t, err)

		nconn1, err := net.Dial("tcp", u1.Host)
		require.NoError(t, err)
		defer nconn1.Close()

		conn1, err := rtmp.NewClientConn(nconn1, u1, true)
		require.NoError(t, err)

		videoTrack := &formats.H264{
			PayloadTyp: 96,
			SPS: []byte{
				0x67, 0x64, 0x00, 0x0c, 0xac, 0x3b, 0x50, 0xb0,
				0x4b, 0x42, 0x00, 0x00, 0x03, 0x00, 0x02, 0x00,
				0x00, 0x03, 0x00, 0x3d, 0x08,
			},
			PPS: []byte{
				0x68, 0xee, 0x3c, 0x80,
			},
			PacketizationMode: 1,
		}

		_, err = rtmp.NewWriter(conn1, videoTrack, nil)
		require.NoError(t, err)

		time.Sleep(500 * time.Millisecond)

		u2, err := url.Parse("rtmp://127.0.0.1:1935/teststream")
		require.NoError(t, err)

		nconn2, err := net.Dial("tcp", u2.Host)
		require.NoError(t, err)
		defer nconn2.Close()

		conn2, err := rtmp.NewClientConn(nconn2, u2, false)
		require.NoError(t, err)

		_, err = rtmp.NewReader(conn2)
		require.EqualError(t, err, "EOF")
	})

	t.Run("read", func(t *testing.T) { //nolint:dupl
		p, ok := newInstance("rtsp: no\n" +
			"hls: no\n" +
			"webrtc: no\n" +
			"paths:\n" +
			"  all:\n" +
			"    readUser: testuser2\n" +
			"    readPass: testpass\n")
		require.Equal(t, true, ok)
		defer p.Close()

		u1, err := url.Parse("rtmp://127.0.0.1:1935/teststream")
		require.NoError(t, err)

		nconn1, err := net.Dial("tcp", u1.Host)
		require.NoError(t, err)
		defer nconn1.Close()

		conn1, err := rtmp.NewClientConn(nconn1, u1, true)
		require.NoError(t, err)

		videoTrack := &formats.H264{
			PayloadTyp: 96,
			SPS: []byte{
				0x67, 0x64, 0x00, 0x0c, 0xac, 0x3b, 0x50, 0xb0,
				0x4b, 0x42, 0x00, 0x00, 0x03, 0x00, 0x02, 0x00,
				0x00, 0x03, 0x00, 0x3d, 0x08,
			},
			PPS: []byte{
				0x68, 0xee, 0x3c, 0x80,
			},
			PacketizationMode: 1,
		}

		_, err = rtmp.NewWriter(conn1, videoTrack, nil)
		require.NoError(t, err)

		time.Sleep(500 * time.Millisecond)

		u2, err := url.Parse("rtmp://127.0.0.1:1935/teststream?user=testuser1&pass=testpass")
		require.NoError(t, err)

		nconn2, err := net.Dial("tcp", u2.Host)
		require.NoError(t, err)
		defer nconn2.Close()

		conn2, err := rtmp.NewClientConn(nconn2, u2, false)
		require.NoError(t, err)

		_, err = rtmp.NewReader(conn2)
		require.EqualError(t, err, "EOF")
	})
}
