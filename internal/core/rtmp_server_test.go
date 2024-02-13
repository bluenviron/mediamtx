package core //nolint:dupl

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp"
	"github.com/bluenviron/mediamtx/internal/test"
)

type testHTTPAuthenticator struct {
	*http.Server
}

func newTestHTTPAuthenticator(t *testing.T, protocol string, action string) *testHTTPAuthenticator {
	firstReceived := false

	ts := &testHTTPAuthenticator{}

	ts.Server = &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "/auth", r.URL.Path)

			var in struct {
				IP       string `json:"ip"`
				User     string `json:"user"`
				Password string `json:"password"`
				Path     string `json:"path"`
				Protocol string `json:"protocol"`
				ID       string `json:"id"`
				Action   string `json:"action"`
				Query    string `json:"query"`
			}
			err := json.NewDecoder(r.Body).Decode(&in)
			require.NoError(t, err)

			var user string
			if action == "publish" {
				user = "testpublisher"
			} else {
				user = "testreader"
			}

			if in.IP != "127.0.0.1" ||
				in.User != user ||
				in.Password != "testpass" ||
				in.Path != "teststream" ||
				in.Protocol != protocol ||
				(firstReceived && in.ID == "") ||
				in.Action != action ||
				(in.Query != "user=testreader&pass=testpass&param=value" &&
					in.Query != "user=testpublisher&pass=testpass&param=value" &&
					in.Query != "param=value") {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			firstReceived = true
		}),
	}

	ln, err := net.Listen("tcp", "127.0.0.1:9120")
	require.NoError(t, err)

	go ts.Server.Serve(ln)

	return ts
}

func (ts *testHTTPAuthenticator) close() {
	ts.Server.Shutdown(context.Background())
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
						"  all_others:\n"

				case "internal":
					conf += "paths:\n" +
						"  all_others:\n" +
						"    publishUser: testpublisher\n" +
						"    publishPass: testpass\n" +
						"    publishIPs: [127.0.0.0/16]\n" +
						"    readUser: testreader\n" +
						"    readPass: testpass\n" +
						"    readIPs: [127.0.0.0/16]\n"

				case "external":
					conf += "externalAuthenticationURL: http://localhost:9120/auth\n" +
						"paths:\n" +
						"  all_others:\n"
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

				w, err := rtmp.NewWriter(conn1, test.FormatH264, test.FormatMPEG4Audio)
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
				require.Equal(t, test.FormatH264, videoTrack1)
				require.Equal(t, test.FormatMPEG4Audio, audioTrack2)

				err = w.WriteH264(0, 0, true, [][]byte{
					{0x05, 0x02, 0x03, 0x04}, // IDR 1
					{0x05, 0x02, 0x03, 0x04}, // IDR 2
				})
				require.NoError(t, err)

				r.OnDataH264(func(pts time.Duration, au [][]byte) {
					require.Equal(t, [][]byte{
						test.FormatH264.SPS,
						test.FormatH264.PPS,
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
			"  all_others:\n" +
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

		videoTrack := &format.H264{
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
			"  all_others:\n")
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

		videoTrack := &format.H264{
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
			"  all_others:\n" +
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

		videoTrack := &format.H264{
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
