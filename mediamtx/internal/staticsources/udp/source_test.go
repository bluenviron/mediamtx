package udp

import (
	"bufio"
	"net"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
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

func TestSource(t *testing.T) {
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
				src = "udp://127.0.0.1:9001"

			case "multicast":
				src = "udp://238.0.0.1:9001"

			case "multicast with interface":
				src = "udp://238.0.0.1:9001?interface=" + multicastCapableInterface(t)

			case "unicast with source":
				src = "udp://127.0.0.1:9001?source=127.0.1.1"
			}

			te := test.NewSourceTester(
				func(p defs.StaticSourceParent) defs.StaticSource {
					return &Source{
						ReadTimeout: conf.Duration(10 * time.Second),
						Parent:      p,
					}
				},
				src,
				&conf.Path{},
			)
			defer te.Close()

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
				Codec: &mpegts.CodecH264{},
			}

			bw := bufio.NewWriter(conn)
			w := &mpegts.Writer{W: bw, Tracks: []*mpegts.Track{track}}
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

			err = bw.Flush()
			require.NoError(t, err)

			<-te.Unit
		})
	}
}
