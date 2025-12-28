package mpegts

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	tscodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts/codecs"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/test"
)

func multicastCapableInterface(t *testing.T) string {
	intfs, err := net.Interfaces()
	require.NoError(t, err)

	for _, intf := range intfs {
		if (intf.Flags & net.FlagMulticast) != 0 {
			return intf.Name
		}
	}

	t.Errorf("unable to find a multicast IP")
	return ""
}

func TestSourceUDP(t *testing.T) {
	for _, ca := range []string{
		"unicast",
		"multicast",
		"multicast with interface",
		"unicast with source",
	} {
		t.Run(ca, func(t *testing.T) {
			var src string

			switch ca {
			case "unicast":
				src = "udp+mpegts://127.0.0.1:9001"

			case "multicast":
				src = "udp+mpegts://238.0.0.1:9001"

			case "multicast with interface":
				src = "udp+mpegts://238.0.0.1:9001?interface=" + multicastCapableInterface(t)

			case "unicast with source":
				src = "udp+mpegts://127.0.0.1:9001?source=127.0.1.1"
			}

			p := &test.StaticSourceParent{}
			p.Initialize()
			defer p.Close()

			so := &Source{
				ReadTimeout: conf.Duration(10 * time.Second),
				Parent:      p,
			}

			done := make(chan struct{})
			defer func() { <-done }()

			ctx, ctxCancel := context.WithCancel(context.Background())
			defer ctxCancel()

			reloadConf := make(chan *conf.Path)

			go func() {
				so.Run(defs.StaticSourceRunParams{ //nolint:errcheck
					Context:        ctx,
					ResolvedSource: src,
					Conf:           &conf.Path{},
					ReloadConf:     reloadConf,
				})
				close(done)
			}()

			time.Sleep(50 * time.Millisecond)

			var dest string

			switch ca {
			case "unicast":
				dest = "127.0.0.1:9001"

			case "multicast":
				dest = "238.0.0.1:9001"

			case "multicast with interface":
				dest = "238.0.0.1:9001"

			case "unicast with source":
				dest = "127.0.0.1:9001"
			}

			udest, err := net.ResolveUDPAddr("udp", dest)
			require.NoError(t, err)

			var usrc *net.UDPAddr
			if ca == "unicast with source" {
				usrc, err = net.ResolveUDPAddr("udp", "127.0.1.1:9020")
				require.NoError(t, err)
			}

			conn, err := net.DialUDP("udp", usrc, udest)
			require.NoError(t, err)
			defer conn.Close() //nolint:errcheck

			track := &mpegts.Track{
				Codec: &tscodecs.H264{},
			}

			w := &mpegts.Writer{W: conn, Tracks: []*mpegts.Track{track}}
			err = w.Initialize()
			require.NoError(t, err)

			err = w.WriteH264(track, 0, 0, [][]byte{{ // IDR
				5, 1,
			}})
			require.NoError(t, err)

			err = w.WriteH264(track, 0, 0, [][]byte{{ // non-IDR
				5, 2,
			}})
			require.NoError(t, err)

			<-p.Unit

			// the source must be listening on ReloadConf
			reloadConf <- nil
		})
	}
}

func TestSourceUnixSocket(t *testing.T) {
	for _, ca := range []string{
		"relative",
		"absolute",
	} {
		t.Run(ca, func(t *testing.T) {
			var pa string
			if ca == "relative" {
				pa = "test_mpegts.sock"
			} else {
				pa = filepath.Join(os.TempDir(), "test_mpegts.sock")
			}

			func() {
				p := &test.StaticSourceParent{}
				p.Initialize()
				defer p.Close()

				so := &Source{
					ReadTimeout: conf.Duration(10 * time.Second),
					Parent:      p,
				}

				done := make(chan struct{})
				defer func() { <-done }()

				ctx, ctxCancel := context.WithCancel(context.Background())
				defer ctxCancel()

				go func() {
					so.Run(defs.StaticSourceRunParams{ //nolint:errcheck
						Context:        ctx,
						ResolvedSource: "unix+mpegts://" + pa,
						Conf:           &conf.Path{},
					})
					close(done)
				}()

				time.Sleep(50 * time.Millisecond)

				_, err := os.Stat(pa)
				require.NoError(t, err)

				conn, err := net.Dial("unix", pa)
				require.NoError(t, err)

				track := &mpegts.Track{
					Codec: &tscodecs.H264{},
				}

				w := &mpegts.Writer{W: conn, Tracks: []*mpegts.Track{track}}
				err = w.Initialize()
				require.NoError(t, err)

				err = w.WriteH264(track, 0, 0, [][]byte{{ // IDR
					5, 1,
				}})
				require.NoError(t, err)

				conn.Close() // trigger a flush

				<-p.Unit
			}()

			_, err := os.Stat(pa)
			require.Error(t, err)
		})
	}
}
