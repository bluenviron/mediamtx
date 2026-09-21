package webrtc

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	pionwebrtc "github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
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
	for _, klv := range []unit.PayloadKLV{
		{},
		{0x06, 0x0e, 0x2b, 0x34},
	} {
		buf := marshalTimedKLV(klv, 0x11223344)

		require.Len(t, buf, timedKLVHeaderSize+len(klv))
		require.Equal(t, uint8(timedKLVEnvelopeVersion), buf[timedKLVVersionOffset])
		require.Equal(t, uint8(timedKLVFlagsNone), buf[timedKLVFlagsOffset])
		require.Equal(t, uint16(timedKLVHeaderSize),
			binary.BigEndian.Uint16(buf[timedKLVHeaderLengthOffset:timedKLVRTPTimeOffset]))
		require.Equal(t, uint32(0x11223344),
			binary.BigEndian.Uint32(buf[timedKLVRTPTimeOffset:timedKLVPayloadOffset]))
		require.Equal(t, []byte(klv), buf[timedKLVPayloadOffset:])
	}
}

type klvDataMessage struct {
	label string
	data  []byte
}

func startKLVTestPeer(
	t *testing.T,
	desc *description.Session,
	klvDataChannelFormat conf.WebRTCKLVDataChannelFormat,
	log logger.Writer,
) (*stream.Reader, chan uint32, chan klvDataMessage) {
	t.Helper()

	client, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() }) //nolint:errcheck

	_, err = client.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo, pionwebrtc.RTPTransceiverInit{
		Direction: pionwebrtc.RTPTransceiverDirectionRecvonly,
	})
	require.NoError(t, err)
	_, err = client.CreateDataChannel("", nil)
	require.NoError(t, err)

	videoTimestamp := make(chan uint32, 3)
	client.OnTrack(func(track *pionwebrtc.TrackRemote, _ *pionwebrtc.RTPReceiver) {
		if track.Kind() != pionwebrtc.RTPCodecTypeVideo {
			return
		}
		go func() {
			for {
				pkt, _, err2 := track.ReadRTP()
				if err2 != nil {
					return
				}
				videoTimestamp <- pkt.Timestamp
			}
		}()
	})

	message := make(chan klvDataMessage, 3)
	client.OnDataChannel(func(dataChannel *pionwebrtc.DataChannel) {
		dataChannel.OnMessage(func(msg pionwebrtc.DataChannelMessage) {
			message <- klvDataMessage{label: dataChannel.Label(), data: msg.Data}
		})
	})

	offer, err := client.CreateOffer(nil)
	require.NoError(t, err)
	require.NoError(t, client.SetLocalDescription(offer))

	server := &PeerConnection{
		LocalRandomUDP:       true,
		IPsFromInterfaces:    true,
		Publish:              true,
		KLVDataChannelFormat: klvDataChannelFormat,
		Log:                  test.NilLogger,
	}
	r := &stream.Reader{Parent: log}
	require.NoError(t, FromStream(desc, r, server))
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
		t.Fatal("timed out waiting for KLV data channel")
	}

	return r, videoTimestamp, message
}

func TestRawKLVDataChannel(t *testing.T) {
	videoFormat := &format.H264{PayloadTyp: 96, PacketizationMode: 1}
	klvFormat := &format.KLV{PayloadTyp: 96}
	videoMedia := &description.Media{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{videoFormat},
	}
	klvMedia := &description.Media{
		Type:    description.MediaTypeApplication,
		Formats: []format.Format{klvFormat},
	}
	desc := &description.Session{Medias: []*description.Media{videoMedia, klvMedia}}

	strm := &stream.Stream{
		OrigDesc:          desc,
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Parent:            test.NilLogger,
	}
	require.NoError(t, strm.Initialize())
	t.Cleanup(strm.Close)

	subStream := &stream.SubStream{Stream: strm}
	require.NoError(t, subStream.Initialize())

	r, _, message := startKLVTestPeer(t, desc, conf.WebRTCKLVDataChannelFormatRaw, test.NilLogger)
	strm.AddReader(r)
	t.Cleanup(func() { strm.RemoveReader(r) })

	klv := unit.PayloadKLV{0x06, 0x0e, 0x2b, 0x34}
	subStream.WriteUnit(klvMedia, klvFormat, &unit.Unit{Payload: klv})

	select {
	case received := <-message:
		require.Equal(t, rawKLVDataChannelLabel, received.label)
		require.Equal(t, []byte(klv), received.data)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for raw KLV message")
	}
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
		Stream: strm,
	}
	require.NoError(t, subStream.Initialize())

	warning := make(chan string, 1)
	r, videoTimestamp, message := startKLVTestPeer(t, strm.OrigDesc,
		conf.WebRTCKLVDataChannelFormatTimed,
		test.Logger(func(level logger.Level, format string, args ...any) {
			if level == logger.Warn {
				warning <- fmt.Sprintf(format, args...)
			}
		}))

	strm.AddReader(r)
	t.Cleanup(func() { strm.RemoveReader(r) })

	baseNTP := time.Unix(1710000000, 0)
	subStream.WriteUnit(klvMedia, klvFormat, &unit.Unit{
		PTS:     89_000,
		NTP:     baseNTP,
		Payload: unit.PayloadKLV{0x00},
	})

	select {
	case msg := <-warning:
		require.Equal(t, "discarding timed KLV messages until a video timestamp is available", msg)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for missing video timestamp warning")
	}

	videoPTS := []int64{90_000, 93_000, 96_000}
	klvDeltas := []uint32{4, 8, 12}
	for i := range videoPTS {
		subStream.WriteUnit(videoMedia, videoFormat, &unit.Unit{
			PTS:     videoPTS[i],
			NTP:     baseNTP,
			Payload: unit.PayloadH264{{0x65, byte(i + 1)}},
		})

		subStream.WriteUnit(klvMedia, klvFormat, &unit.Unit{
			PTS:     videoPTS[i] + int64(klvDeltas[i]),
			NTP:     baseNTP,
			Payload: unit.PayloadKLV{0x06, 0x0e, 0x2b, 0x34, byte(i)},
		})
	}

	receivedVideoTimestamps := make([]uint32, len(videoPTS))
	for i := range receivedVideoTimestamps {
		select {
		case receivedVideoTimestamps[i] = <-videoTimestamp:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for video RTP packet")
		}
	}

	for range videoPTS {
		select {
		case received := <-message:
			require.Equal(t, timedKLVDataChannelLabel, received.label)
			require.GreaterOrEqual(t, len(received.data), timedKLVHeaderSize)
			require.Equal(t, uint8(timedKLVEnvelopeVersion), received.data[timedKLVVersionOffset])
			headerLength := int(binary.BigEndian.Uint16(
				received.data[timedKLVHeaderLengthOffset:timedKLVRTPTimeOffset]))
			require.GreaterOrEqual(t, headerLength, timedKLVHeaderSize)
			require.LessOrEqual(t, headerLength, len(received.data))
			klv := received.data[headerLength:]
			require.Len(t, klv, 5)
			index := int(klv[4])
			require.Less(t, index, len(videoPTS))
			require.Equal(t,
				receivedVideoTimestamps[index]+klvDeltas[index],
				binary.BigEndian.Uint32(received.data[timedKLVRTPTimeOffset:timedKLVPayloadOffset]))
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for timed KLV message")
		}
	}

	select {
	case msg := <-warning:
		require.Equal(t, "timed KLV messages discarded before a video timestamp became available: 1", msg)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for discarded KLV summary")
	}
}
