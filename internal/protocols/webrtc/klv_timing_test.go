package webrtc

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"
	pionwebrtc "github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func TestVideoTimestampMapper(t *testing.T) {
	var mapper videoTimestampMapper

	_, ok := mapper.translate(90_000)
	require.False(t, ok)

	mapper.update(90_000, 0xfffffff0)

	timestamp, ok := mapper.translate(90_032)
	require.True(t, ok)
	require.Equal(t, uint32(0x10), timestamp)

	timestamp, ok = mapper.translate(89_984)
	require.True(t, ok)
	require.Equal(t, uint32(0xffffffe0), timestamp)
}

func TestMarshalTimedKLV(t *testing.T) {
	klv := unit.PayloadKLV{0x06, 0x0e, 0x2b, 0x34}

	buf := marshalTimedKLV(klv, 0x11223344)

	require.Equal(t, uint8(1), buf[0])
	require.Equal(t, uint8(0), buf[1])
	require.Equal(t, uint16(8), binary.BigEndian.Uint16(buf[2:4]))
	require.Equal(t, uint32(0x11223344), binary.BigEndian.Uint32(buf[4:8]))
	require.Equal(t, []byte{0x06, 0x0e, 0x2b, 0x34}, buf[8:])
}

func TestTimedKLVDataChannel(t *testing.T) {
	videoFormat := &format.H264{
		PayloadTyp:        96,
		PacketizationMode: 1,
	}
	klvFormat := &format.KLV{PayloadTyp: 96}
	videoMedia := &description.Media{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{videoFormat},
	}
	klvMedia := &description.Media{
		Type:    description.MediaTypeApplication,
		Formats: []format.Format{klvFormat},
	}

	strm := &stream.Stream{
		OrigDesc:          &description.Session{Medias: []*description.Media{videoMedia, klvMedia}},
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		ReplaceNTP:        false,
		Parent:            test.NilLogger,
	}
	require.NoError(t, strm.Initialize())
	t.Cleanup(strm.Close)

	subStream := &stream.SubStream{
		Stream:        strm,
		UseRTPPackets: true,
	}
	require.NoError(t, subStream.Initialize())

	client, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() }) //nolint:errcheck

	_, err = client.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo, pionwebrtc.RTPTransceiverInit{
		Direction: pionwebrtc.RTPTransceiverDirectionRecvonly,
	})
	require.NoError(t, err)
	_, err = client.CreateDataChannel("", nil)
	require.NoError(t, err)

	videoTimestamp := make(chan uint32, 1)
	client.OnTrack(func(track *pionwebrtc.TrackRemote, _ *pionwebrtc.RTPReceiver) {
		if track.Kind() != pionwebrtc.RTPCodecTypeVideo {
			return
		}
		go func() {
			pkt, _, err2 := track.ReadRTP()
			if err2 == nil {
				videoTimestamp <- pkt.Timestamp
			}
		}()
	})

	type dataMessage struct {
		label string
		data  []byte
	}
	message := make(chan dataMessage, 1)
	client.OnDataChannel(func(dataChannel *pionwebrtc.DataChannel) {
		dataChannel.OnMessage(func(msg pionwebrtc.DataChannelMessage) {
			message <- dataMessage{label: dataChannel.Label(), data: msg.Data}
		})
	})

	offer, err := client.CreateOffer(nil)
	require.NoError(t, err)
	require.NoError(t, client.SetLocalDescription(offer))

	server := &PeerConnection{
		LocalRandomUDP:    true,
		IPsFromInterfaces: true,
		Publish:           true,
		Log:               test.NilLogger,
	}
	r := &stream.Reader{Parent: test.NilLogger}
	require.NoError(t, FromStream(strm.OrigDesc, r, server, conf.WebRTCKLVDataChannelFormatTimed))
	require.NoError(t, server.Start())
	t.Cleanup(server.Close)
	dataChannelOpened := make(chan struct{})
	server.OutboundDataChannels[0].dataChan.OnOpen(func() {
		close(dataChannelOpened)
	})

	answer, err := server.CreateFullAnswer(&offer, false)
	require.NoError(t, err)
	require.NoError(t, client.SetRemoteDescription(*answer))
	require.NoError(t, server.WaitUntilConnected(10*time.Second))
	select {
	case <-dataChannelOpened:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for timed KLV data channel")
	}

	strm.AddReader(r)
	t.Cleanup(func() { strm.RemoveReader(r) })

	const videoPTS = int64(90_000)
	const inputVideoTimestamp = uint32(0x11223344)
	baseNTP := time.Unix(1710000000, 0)
	subStream.WriteUnit(videoMedia, videoFormat, &unit.Unit{
		PTS: videoPTS,
		NTP: baseNTP,
		RTPPackets: []*rtp.Packet{{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: 1,
				Timestamp:      inputVideoTimestamp,
				SSRC:           1,
			},
			Payload: []byte{0x65, 0x01},
		}},
		Payload: unit.PayloadH264{{0x65, 0x01}},
	})

	klv := unit.PayloadKLV{0x06, 0x0e, 0x2b, 0x34}
	subStream.WriteUnit(klvMedia, klvFormat, &unit.Unit{
		PTS:     videoPTS + 4,
		NTP:     baseNTP,
		Payload: klv,
	})

	var receivedVideoTimestamp uint32
	select {
	case receivedVideoTimestamp = <-videoTimestamp:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for video RTP packet")
	}

	select {
	case received := <-message:
		require.Equal(t, timedKLVDataChannelLabel, received.label)
		require.Equal(t, uint8(timedKLVEnvelopeVersion), received.data[0])
		require.Equal(t, uint16(timedKLVHeaderSize), binary.BigEndian.Uint16(received.data[2:4]))
		require.Equal(t, receivedVideoTimestamp+4, binary.BigEndian.Uint32(received.data[4:8]))
		require.Equal(t, []byte(klv), received.data[timedKLVHeaderSize:])
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for timed KLV message")
	}
}
