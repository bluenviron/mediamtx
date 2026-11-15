package rtp_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	forwardrtp "github.com/bluenviron/mediamtx/internal/forward/rtp"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func newStream(t *testing.T, desc *description.Session) (*stream.Stream, *stream.SubStream) {
	t.Helper()

	strm := &stream.Stream{
		OrigDesc:          desc,
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Parent:            test.NilLogger,
	}
	require.NoError(t, strm.Initialize())
	t.Cleanup(strm.Close)

	subStream := &stream.SubStream{
		Stream:        strm,
		UseRTPPackets: true,
	}
	require.NoError(t, subStream.Initialize())

	return strm, subStream
}

func newPacket(payloadType uint8, payload []byte) *rtp.Packet {
	pkt := &rtp.Packet{Payload: payload}
	pkt.Version = 2
	pkt.PayloadType = payloadType
	pkt.SequenceNumber = 1
	return pkt
}

func TestDest(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer pc.Close()

	videoFormat := &format.Generic{PayloadTyp: 96, RTPMa: "private/90000"}
	require.NoError(t, videoFormat.Init())

	audioFormat := &format.Generic{PayloadTyp: 97, RTPMa: "private/48000"}
	require.NoError(t, audioFormat.Init())

	desc := &description.Session{Medias: []*description.Media{
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{videoFormat},
		},
		{
			Type:    description.MediaTypeAudio,
			Formats: []format.Format{audioFormat},
		},
	}}

	strm, subStream := newStream(t, desc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dest := &forwardrtp.Dest{
		Stream:       strm,
		Dest:         "udp+rtp://" + pc.LocalAddr().String(),
		WriteTimeout: conf.Duration(10 * time.Second),
		Parent:       test.NilLogger,
	}

	done := make(chan error, 1)
	go func() {
		done <- dest.Run(ctx)
	}()

	strm.WaitForReaders()

	subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
		PTS:        0,
		RTPPackets: []*rtp.Packet{newPacket(96, []byte{5, 1})},
	})
	subStream.WriteUnit(desc.Medias[1], desc.Medias[1].Formats[0], &unit.Unit{
		PTS:        0,
		RTPPackets: []*rtp.Packet{newPacket(97, []byte{1, 2})},
	})

	received := map[uint8][]byte{}

	for len(received) < 2 {
		buf := make([]byte, 1500)
		require.NoError(t, pc.SetReadDeadline(time.Now().Add(5*time.Second)))
		n, _, readErr := pc.ReadFrom(buf)
		require.NoError(t, readErr)

		var pkt rtp.Packet
		require.NoError(t, pkt.Unmarshal(buf[:n]))
		received[pkt.PayloadType] = pkt.Payload
	}

	require.Equal(t, map[uint8][]byte{96: {5, 1}, 97: {1, 2}}, received)

	require.Eventually(t, func() bool {
		return dest.Info().OutboundBytes > 0
	}, 5*time.Second, 10*time.Millisecond)

	cancel()
	select {
	case runErr := <-done:
		require.EqualError(t, runErr, "terminated")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for RTP destination to stop")
	}
}

func TestDestPayloadTypeCollision(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.Generic{PayloadTyp: 96, RTPMa: "private/90000", ClockRat: 90000}},
		},
		{
			Type:    description.MediaTypeAudio,
			Formats: []format.Format{&format.Generic{PayloadTyp: 96, RTPMa: "private/48000", ClockRat: 48000}},
		},
	}}

	strm, _ := newStream(t, desc)

	dest := &forwardrtp.Dest{
		Stream:       strm,
		Dest:         "udp+rtp://127.0.0.1:9",
		WriteTimeout: conf.Duration(10 * time.Second),
		Parent:       test.NilLogger,
	}

	err := dest.Run(context.Background())
	require.EqualError(t, err, "payload type 96 is used by more than one format")
}

func TestDestInterfaceWithUnicast(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{test.FormatH264},
	}}}

	strm, _ := newStream(t, desc)

	dest := &forwardrtp.Dest{
		Stream:       strm,
		Dest:         "udp+rtp://127.0.0.1:9?interface=lo",
		WriteTimeout: conf.Duration(10 * time.Second),
		Parent:       test.NilLogger,
	}

	err := dest.Run(context.Background())
	require.EqualError(t, err, "parameter 'interface' can be used only with multicast addresses")
}
