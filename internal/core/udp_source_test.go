package core

import (
	"bufio"
	"net"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v3"
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/url"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestUDPSource(t *testing.T) {
	p, ok := newInstance("paths:\n" +
		"  proxied:\n" +
		"    source: udp://localhost:9999\n" +
		"    sourceOnDemand: yes\n")
	require.Equal(t, true, ok)
	defer p.Close()

	c := gortsplib.Client{}

	u, err := url.Parse("rtsp://127.0.0.1:8554/proxied")
	require.NoError(t, err)

	err = c.Start(u.Scheme, u.Host)
	require.NoError(t, err)
	defer c.Close()

	connected := make(chan struct{})
	received := make(chan struct{})

	go func() {
		time.Sleep(200 * time.Millisecond)

		conn, err := net.Dial("udp", "localhost:9999")
		require.NoError(t, err)
		defer conn.Close()

		track := &mpegts.Track{
			Codec: &mpegts.CodecH264{},
		}

		bw := bufio.NewWriter(conn)
		w := mpegts.NewWriter(bw, []*mpegts.Track{track})
		require.NoError(t, err)

		err = w.WriteH26x(track, 0, 0, true, [][]byte{
			{ // IDR
				0x05, 1,
			},
		})
		require.NoError(t, err)

		err = bw.Flush()
		require.NoError(t, err)

		<-connected

		err = w.WriteH26x(track, 0, 0, true, [][]byte{{5, 2}})
		require.NoError(t, err)

		err = bw.Flush()
		require.NoError(t, err)
	}()

	medias, baseURL, _, err := c.Describe(u)
	require.NoError(t, err)

	var forma *formats.H264
	medi := medias.FindFormat(&forma)

	_, err = c.Setup(medi, baseURL, 0, 0)
	require.NoError(t, err)

	c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
		require.Equal(t, []byte{5, 1}, pkt.Payload)
		close(received)
	})

	_, err = c.Play(nil)
	require.NoError(t, err)

	close(connected)
	<-received
}
