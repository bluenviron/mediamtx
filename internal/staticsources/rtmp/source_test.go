package rtmp

import (
	"crypto/tls"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestSource(t *testing.T) {
	for _, ca := range []string{
		"plain",
		"tls",
	} {
		t.Run(ca, func(t *testing.T) {
			ln, err := func() (net.Listener, error) {
				if ca == "plain" {
					return net.Listen("tcp", "127.0.0.1:1935")
				}

				serverCertFpath, err := test.CreateTempFile(test.TLSCertPub)
				require.NoError(t, err)
				defer os.Remove(serverCertFpath)

				serverKeyFpath, err := test.CreateTempFile(test.TLSCertKey)
				require.NoError(t, err)
				defer os.Remove(serverKeyFpath)

				var cert tls.Certificate
				cert, err = tls.LoadX509KeyPair(serverCertFpath, serverKeyFpath)
				require.NoError(t, err)

				return tls.Listen("tcp", "127.0.0.1:1936", &tls.Config{Certificates: []tls.Certificate{cert}})
			}()
			require.NoError(t, err)
			defer ln.Close()

			go func() {
				nconn, err := ln.Accept()
				require.NoError(t, err)
				defer nconn.Close()

				conn := &rtmp.Conn{
					RW: nconn,
				}
				err = conn.Initialize()
				require.NoError(t, err)

				w := &rtmp.Writer{
					Conn:       conn,
					VideoTrack: test.FormatH264,
					AudioTrack: test.FormatMPEG4Audio,
				}
				err = w.Initialize()
				require.NoError(t, err)

				err = w.WriteH264(2*time.Second, 2*time.Second, [][]byte{{5, 2, 3, 4}})
				require.NoError(t, err)

				err = w.WriteH264(3*time.Second, 3*time.Second, [][]byte{{5, 2, 3, 4}})
				require.NoError(t, err)
			}()

			var te *test.SourceTester

			if ca == "plain" {
				te = test.NewSourceTester(
					func(p defs.StaticSourceParent) defs.StaticSource {
						return &Source{
							ReadTimeout:  conf.Duration(10 * time.Second),
							WriteTimeout: conf.Duration(10 * time.Second),
							Parent:       p,
						}
					},
					"rtmp://localhost/teststream",
					&conf.Path{},
				)
			} else {
				te = test.NewSourceTester(
					func(p defs.StaticSourceParent) defs.StaticSource {
						return &Source{
							ReadTimeout:  conf.Duration(10 * time.Second),
							WriteTimeout: conf.Duration(10 * time.Second),
							Parent:       p,
						}
					},
					"rtmps://localhost/teststream",
					&conf.Path{
						SourceFingerprint: "33949E05FFFB5FF3E8AA16F8213A6251B4D9363804BA53233C4DA9A46D6F2739",
					},
				)
			}

			defer te.Close()

			<-te.Unit
		})
	}
}
