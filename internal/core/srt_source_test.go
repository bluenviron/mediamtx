package core

import (
	"bufio"
	"testing"

	"github.com/bluenviron/gortsplib/v3"
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/url"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
	"github.com/datarhei/gosrt"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestSRTSource(t *testing.T) {
	ln, err := srt.Listen("srt", "localhost:9999", srt.DefaultConfig())
	require.NoError(t, err)
	defer ln.Close()

	connected := make(chan struct{})
	received := make(chan struct{})
	done := make(chan struct{})

	go func() {
		conn, _, err := ln.Accept(func(req srt.ConnRequest) srt.ConnType {
			require.Equal(t, "sidname", req.StreamId())

			err := req.SetPassphrase("ttest1234567")
			if err != nil {
				return srt.REJECT
			}

			return srt.SUBSCRIBE
		})
		require.NoError(t, err)
		require.NotNil(t, conn)
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

		<-done
	}()

	p, ok := newInstance("paths:\n" +
		"  proxied:\n" +
		"    source: srt://localhost:9999?streamid=sidname&passphrase=ttest1234567\n" +
		"    sourceOnDemand: yes\n")
	require.Equal(t, true, ok)
	defer p.Close()

	c := gortsplib.Client{}

	u, err := url.Parse("rtsp://127.0.0.1:8554/proxied")
	require.NoError(t, err)

	err = c.Start(u.Scheme, u.Host)
	require.NoError(t, err)
	defer c.Close()

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
	close(done)
}
