package rtp

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/format/rtph264"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/test"
)

const testRTPSDP = "v=0\n" +
	"o=- 123456789 123456789 IN IP4 192.168.1.100\n" +
	"s=H264 Video Stream\n" +
	"c=IN IP4 192.168.1.100\n" +
	"t=0 0\n" +
	"m=video 5004 RTP/AVP 96\n" +
	"a=rtpmap:96 H264/90000\n" +
	"a=fmtp:96 profile-level-id=42e01e;packetization-mode=1\n"

// a fresh source, before connecting, reports nil stats.
func TestSourceStatsEmpty(t *testing.T) {
	so := &Source{}
	require.Nil(t, so.SourceStats())
}

func TestSourceStats(t *testing.T) {
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
			ResolvedSource: "udp+rtp://127.0.0.1:9006",
			Conf:           &conf.Path{RTPSDP: testRTPSDP},
			ReloadConf:     reloadConf,
		})
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	udest, err := net.ResolveUDPAddr("udp", "127.0.0.1:9006")
	require.NoError(t, err)

	conn, err := net.DialUDP("udp", nil, udest)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	enc := &rtph264.Encoder{
		PayloadType:       96,
		PacketizationMode: 1,
	}
	require.NoError(t, enc.Init())

	pkts, err := enc.Encode([][]byte{{5, 1}})
	require.NoError(t, err)

	for _, pkt := range pkts {
		var buf []byte
		buf, err = pkt.Marshal()
		require.NoError(t, err)

		_, err = conn.Write(buf)
		require.NoError(t, err)
	}

	<-p.Unit

	st := so.SourceStats()
	require.NotNil(t, st)

	rst, ok := st.(*defs.RTPSourceStats)
	require.True(t, ok)
	require.Equal(t, uint64(len(pkts)), rst.PacketsReceived)
	require.Equal(t, uint64(0), rst.PacketsLost)
	require.Nil(t, rst.Jitter)

	// the source must be listening on ReloadConf
	reloadConf <- nil
}
