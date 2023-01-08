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

type webRTCTestClient struct {
	wc     *websocket.Conn
	pc     *webrtc.PeerConnection
	track  chan *webrtc.TrackRemote
	closed chan struct{}
}

func newWebRTCTestClient(addr string) (*webRTCTestClient, error) {
	wc, res, err := websocket.DefaultDialer.Dial(addr, nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	_, msg, err := wc.ReadMessage()
	if err != nil {
		wc.Close()
		return nil, err
	}

	var iceServers []webrtc.ICEServer
	err = json.Unmarshal(msg, &iceServers)
	if err != nil {
		wc.Close()
		return nil, err
	}

	pc, err := newPeerConnection(webrtc.Configuration{
		ICEServers: iceServers,
	})
	if err != nil {
		wc.Close()
		return nil, err
	}

	pc.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i != nil {
			enc, _ := json.Marshal(i.ToJSON())
			wc.WriteMessage(websocket.TextMessage, enc)
		}
	})

	connected := make(chan struct{})
	closed := make(chan struct{})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		switch state {
		case webrtc.PeerConnectionStateConnected:
			close(connected)

		case webrtc.PeerConnectionStateClosed:
			select {
			case <-closed:
				return
			default:
			}
			close(closed)
		}
	})

	track := make(chan *webrtc.TrackRemote, 1)

	pc.OnTrack(func(trak *webrtc.TrackRemote, recv *webrtc.RTPReceiver) {
		track <- trak
	})

	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	if err != nil {
		wc.Close()
		pc.Close()
		return nil, err
	}

	localOffer, err := pc.CreateOffer(nil)
	if err != nil {
		wc.Close()
		pc.Close()
		return nil, err
	}

	enc, err := json.Marshal(localOffer)
	if err != nil {
		wc.Close()
		pc.Close()
		return nil, err
	}

	err = wc.WriteMessage(websocket.TextMessage, enc)
	if err != nil {
		wc.Close()
		pc.Close()
		return nil, err
	}

	err = pc.SetLocalDescription(localOffer)
	if err != nil {
		wc.Close()
		pc.Close()
		return nil, err
	}

	_, msg, err = wc.ReadMessage()
	if err != nil {
		wc.Close()
		pc.Close()
		return nil, err
	}

	var remoteOffer webrtc.SessionDescription
	err = json.Unmarshal(msg, &remoteOffer)
	if err != nil {
		wc.Close()
		pc.Close()
		return nil, err
	}

	err = pc.SetRemoteDescription(remoteOffer)
	if err != nil {
		wc.Close()
		pc.Close()
		return nil, err
	}

	go func() {
		for {
			_, msg, err := wc.ReadMessage()
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

	return &webRTCTestClient{
		wc:     wc,
		pc:     pc,
		track:  track,
		closed: closed,
	}, nil
}

func (c *webRTCTestClient) close() {
	c.pc.Close()
	c.wc.Close()
	<-c.closed
}

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

	c, err := newWebRTCTestClient("ws://localhost:8889/stream/ws")
	require.NoError(t, err)
	defer c.close()

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

	trak := <-c.track

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
