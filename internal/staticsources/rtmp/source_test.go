package rtmp

import (
	"context"
	"crypto/tls"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/gortmplib"
	"github.com/bluenviron/gortmplib/pkg/codecs"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestSource(t *testing.T) {
	for _, encryption := range []string{
		"plain",
		"tls",
	} {
		for _, auth := range []string{
			"no auth",
			"auth",
		} {
			t.Run(encryption+"_"+auth, func(t *testing.T) {
				var ln net.Listener

				if encryption == "plain" {
					var err error
					ln, err = net.Listen("tcp", "127.0.0.1:1935")
					require.NoError(t, err)
				} else {
					serverCertFpath, err := test.CreateTempFile(test.TLSCertPub)
					require.NoError(t, err)
					defer os.Remove(serverCertFpath)

					serverKeyFpath, err := test.CreateTempFile(test.TLSCertKey)
					require.NoError(t, err)
					defer os.Remove(serverKeyFpath)

					var cert tls.Certificate
					cert, err = tls.LoadX509KeyPair(serverCertFpath, serverKeyFpath)
					require.NoError(t, err)

					ln, err = tls.Listen("tcp", "127.0.0.1:1936", &tls.Config{Certificates: []tls.Certificate{cert}})
					require.NoError(t, err)
				}

				defer ln.Close()

				var source string

				if encryption == "plain" {
					source = "rtmp://"
				} else {
					source = "rtmps://"
				}

				if auth == "auth" {
					source += "myuser:mypass@"
				}

				source += "localhost/teststream"

				p := &test.StaticSourceParent{}
				p.Initialize()
				defer p.Close()

				so := &Source{
					ReadTimeout:  conf.Duration(10 * time.Second),
					WriteTimeout: conf.Duration(10 * time.Second),
					Parent:       p,
				}

				done := make(chan struct{})
				defer func() { <-done }()

				ctx, ctxCancel := context.WithCancel(context.Background())
				defer ctxCancel()

				reloadConf := make(chan *conf.Path)

				go func() {
					so.Run(defs.StaticSourceRunParams{ //nolint:errcheck
						Context:        ctx,
						ResolvedSource: source,
						Conf: &conf.Path{
							SourceFingerprint: "33949E05FFFB5FF3E8AA16F8213A6251B4D9363804BA53233C4DA9A46D6F2739",
						},
						ReloadConf: reloadConf,
					})
					close(done)
				}()

				for {
					nconn, err := ln.Accept()
					require.NoError(t, err)
					defer nconn.Close()

					conn := &gortmplib.ServerConn{
						RW: nconn,
					}
					err = conn.Initialize()
					require.NoError(t, err)

					if auth == "auth" {
						err = conn.CheckCredentials("myuser", "mypass")
						if err != nil {
							continue
						}
					}

					err = conn.Accept()
					require.NoError(t, err)

					w := &gortmplib.Writer{
						Conn: conn,
						Tracks: []*gortmplib.Track{
							{Codec: &codecs.H264{
								SPS: test.FormatH264.SPS,
								PPS: test.FormatH264.PPS,
							}},
							{Codec: &codecs.MPEG4Audio{
								Config: test.FormatMPEG4Audio.Config,
							}},
						},
					}
					err = w.Initialize()
					require.NoError(t, err)

					err = w.WriteMPEG4Audio(w.Tracks[1], 2*time.Second, []byte{5, 2, 3, 4})
					require.NoError(t, err)

					break
				}

				<-p.Unit

				// the source must be listening on ReloadConf
				reloadConf <- nil
			})
		}
	}
}
