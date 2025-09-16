package rtp

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/format/rtph264"
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
				src = "udp+rtp://127.0.0.1:9004"

			case "multicast":
				src = "udp+rtp://238.0.0.1:9004"

			case "multicast with interface":
				src = "udp+rtp://238.0.0.1:9004?interface=" + multicastCapableInterface(t)

			case "unicast with source":
				src = "udp+rtp://127.0.0.1:9004?source=127.0.1.1"
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
					Conf: &conf.Path{
						RTPSDP: "v=0\n" +
							"o=- 123456789 123456789 IN IP4 192.168.1.100\n" +
							"s=H264 Video Stream\n" +
							"c=IN IP4 192.168.1.100\n" +
							"t=0 0\n" +
							"m=video 5004 RTP/AVP 96\n" +
							"a=rtpmap:96 H264/90000\n" +
							"a=fmtp:96 profile-level-id=42e01e;packetization-mode=1\n",
					},
					ReloadConf: reloadConf,
				})
				close(done)
			}()

			time.Sleep(50 * time.Millisecond)

			var dest string

			switch ca {
			case "unicast":
				dest = "127.0.0.1:9004"

			case "multicast":
				dest = "238.0.0.1:9004"

			case "multicast with interface":
				dest = "238.0.0.1:9004"

			case "unicast with source":
				dest = "127.0.0.1:9004"
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

			enc := &rtph264.Encoder{
				PayloadType: 96,
			}
			err = enc.Init()
			require.NoError(t, err)

			pkts, err := enc.Encode([][]byte{
				{5, 1},
			})
			require.NoError(t, err)

			for _, pkt := range pkts {
				var buf []byte
				buf, err = pkt.Marshal()
				require.NoError(t, err)

				_, err = conn.Write(buf)
				require.NoError(t, err)
			}

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
				pa = "test_rtp.sock"
			} else {
				pa = filepath.Join(os.TempDir(), "test_rtp.sock")
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
						ResolvedSource: "unix+rtp://" + pa,
						Conf: &conf.Path{
							RTPSDP: "v=0\n" +
								"o=- 123456789 123456789 IN IP4 192.168.1.100\n" +
								"s=H264 Video Stream\n" +
								"c=IN IP4 192.168.1.100\n" +
								"t=0 0\n" +
								"m=video 5004 RTP/AVP 96\n" +
								"a=rtpmap:96 H264/90000\n" +
								"a=fmtp:96 profile-level-id=42e01e;packetization-mode=1\n",
						},
					})
					close(done)
				}()

				time.Sleep(50 * time.Millisecond)

				_, err := os.Stat(pa)
				require.NoError(t, err)

				conn, err := net.Dial("unix", pa)
				require.NoError(t, err)
				defer conn.Close()

				enc := &rtph264.Encoder{
					PayloadType: 96,
				}
				err = enc.Init()
				require.NoError(t, err)

				pkts, err := enc.Encode([][]byte{
					{5, 1},
				})
				require.NoError(t, err)

				for _, pkt := range pkts {
					var buf []byte
					buf, err = pkt.Marshal()
					require.NoError(t, err)

					_, err = conn.Write(buf)
					require.NoError(t, err)
				}

				<-p.Unit
			}()

			_, err := os.Stat(pa)
			require.Error(t, err)
		})
	}
}
