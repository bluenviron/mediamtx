package core

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/aler9/gortsplib/v2"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/media"
	"github.com/gorilla/websocket"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
)

func TestWebRTCServer(t *testing.T) {
	p, ok := newInstance("paths:\n" +
		"  all:\n")
	require.Equal(t, true, ok)
	defer p.Close()

	medi := &media.Media{
		Type: media.TypeVideo,
		Formats: []format.Format{&format.H264{
			PayloadTyp:        96,
			PacketizationMode: 1,
		}},
	}

	v := gortsplib.TransportTCP
	source := gortsplib.Client{
		Transport: &v,
	}
	err := source.StartRecording("rtsp://localhost:8554/stream", media.Medias{medi})
	require.NoError(t, err)
	defer source.Close()

	c, _, err := websocket.DefaultDialer.Dial("ws://localhost:8889/stream/ws", nil) //nolint:bodyclose
	require.NoError(t, err)
	defer c.Close()

	_, msg, err := c.ReadMessage()
	require.NoError(t, err)

	var iceServers []webrtc.ICEServer
	err = json.Unmarshal(msg, &iceServers)
	require.NoError(t, err)

	pc, err := newPeerConnection(webrtc.Configuration{
		ICEServers: iceServers,
	})
	require.NoError(t, err)
	defer pc.Close()

	pc.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i != nil {
			enc, _ := json.Marshal(i.ToJSON())
			c.WriteMessage(websocket.TextMessage, enc)
		}
	})

	connected := make(chan struct{})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateConnected {
			close(connected)
		}
	})

	track := make(chan *webrtc.TrackRemote)
	pc.OnTrack(func(trak *webrtc.TrackRemote, recv *webrtc.RTPReceiver) {
		track <- trak
	})

	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	require.NoError(t, err)

	localOffer, err := pc.CreateOffer(nil)
	require.NoError(t, err)

	enc, err := json.Marshal(localOffer)
	require.NoError(t, err)

	err = c.WriteMessage(websocket.TextMessage, enc)
	require.NoError(t, err)

	err = pc.SetLocalDescription(localOffer)
	require.NoError(t, err)

	_, msg, err = c.ReadMessage()
	require.NoError(t, err)

	var remoteOffer webrtc.SessionDescription
	err = json.Unmarshal(msg, &remoteOffer)
	require.NoError(t, err)

	err = pc.SetRemoteDescription(remoteOffer)
	require.NoError(t, err)

	go func() {
		for {
			_, msg, err := c.ReadMessage()
			if err != nil {
				return
			}

			var candidate webrtc.ICECandidateInit
			err = json.Unmarshal(msg, &candidate)
			if err != nil {
				return
			}

			pc.AddICECandidate(candidate)
		}
	}()

	<-connected

	time.Sleep(500 * time.Millisecond)

	source.WritePacketRTP(medi, &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         true,
			PayloadType:    96,
			SequenceNumber: 123,
			Timestamp:      45343,
			SSRC:           563423,
		},
		Payload: []byte{0x01, 0x02, 0x03, 0x04},
	})

	trak := <-track

	pkt, _, err := trak.ReadRTP()
	require.NoError(t, err)
	require.Equal(t, &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         true,
			PayloadType:    102,
			SequenceNumber: pkt.SequenceNumber,
			Timestamp:      pkt.Timestamp,
			SSRC:           pkt.SSRC,
			CSRC:           []uint32{},
		},
		Payload: []byte{0x01, 0x02, 0x03, 0x04},
	}, pkt)
}
