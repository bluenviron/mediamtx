package core

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v3"
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/bluenviron/gortsplib/v3/pkg/url"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
)

func whipGetICEServers(t *testing.T, ur string) []webrtc.ICEServer {
	req, err := http.NewRequest("OPTIONS", ur, nil)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)

	link, ok := res.Header["Link"]
	require.Equal(t, true, ok)
	servers := linkHeaderToIceServers(link)
	require.NotEqual(t, 0, len(servers))

	return servers
}

func whipPostOffer(t *testing.T, ur string, offer *webrtc.SessionDescription) (*webrtc.SessionDescription, string) {
	enc, err := json.Marshal(offer)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", ur, bytes.NewReader(enc))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/sdp")

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusCreated, res.StatusCode)

	link, ok := res.Header["Link"]
	require.Equal(t, true, ok)
	servers := linkHeaderToIceServers(link)
	require.NotEqual(t, 0, len(servers))

	require.Equal(t, "application/sdp", res.Header.Get("Content-Type"))
	etag := res.Header.Get("E-Tag")
	require.NotEqual(t, 0, len(etag))
	require.Equal(t, "application/trickle-ice-sdpfrag", res.Header.Get("Accept-Patch"))

	var answer webrtc.SessionDescription
	err = json.NewDecoder(res.Body).Decode(&answer)
	require.NoError(t, err)

	return &answer, etag
}

func whipPostCandidate(t *testing.T, ur string, offer *webrtc.SessionDescription,
	etag string, candidate *webrtc.ICECandidateInit,
) {
	frag, err := marshalICEFragment(offer, []*webrtc.ICECandidateInit{candidate})
	require.NoError(t, err)

	req, err := http.NewRequest("PATCH", ur, bytes.NewReader(frag))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/trickle-ice-sdpfrag")
	req.Header.Set("If-Match", etag)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNoContent, res.StatusCode)
}

type webRTCTestClient struct {
	pc             *webrtc.PeerConnection
	outgoingTrack1 *webrtc.TrackLocalStaticRTP
	outgoingTrack2 *webrtc.TrackLocalStaticRTP
	incomingTrack  chan *webrtc.TrackRemote
	closed         chan struct{}
}

func newWebRTCTestClient(t *testing.T, ur string, publish bool) *webRTCTestClient {
	iceServers := whipGetICEServers(t, ur)

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: iceServers,
	})
	require.NoError(t, err)

	connected := make(chan struct{})
	closed := make(chan struct{})
	var stateChangeMutex sync.Mutex

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		stateChangeMutex.Lock()
		defer stateChangeMutex.Unlock()

		select {
		case <-closed:
			return
		default:
		}

		switch state {
		case webrtc.PeerConnectionStateConnected:
			close(connected)

		case webrtc.PeerConnectionStateClosed:
			close(closed)
		}
	})

	var outgoingTrack1 *webrtc.TrackLocalStaticRTP
	var outgoingTrack2 *webrtc.TrackLocalStaticRTP
	var incomingTrack chan *webrtc.TrackRemote

	if publish {
		var err error
		outgoingTrack1, err = webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP8,
				ClockRate: 90000,
			},
			"vp8",
			webrtcStreamID,
		)
		require.NoError(t, err)

		_, err = pc.AddTrack(outgoingTrack1)
		require.NoError(t, err)

		outgoingTrack2, err = webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeOpus,
				ClockRate: 48000,
				Channels:  2,
			},
			"opus",
			webrtcStreamID,
		)
		require.NoError(t, err)

		_, err = pc.AddTrack(outgoingTrack2)
		require.NoError(t, err)
	} else {
		incomingTrack = make(chan *webrtc.TrackRemote, 1)
		pc.OnTrack(func(trak *webrtc.TrackRemote, recv *webrtc.RTPReceiver) {
			incomingTrack <- trak
		})

		_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
		require.NoError(t, err)
	}

	offer, err := pc.CreateOffer(nil)
	require.NoError(t, err)

	answer, etag := whipPostOffer(t, ur, &offer)

	// test adding additional candidates, even if it is not mandatory here
	gatheringDone := make(chan struct{})
	pc.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i != nil {
			c := i.ToJSON()
			whipPostCandidate(t, ur, &offer, etag, &c)
		} else {
			close(gatheringDone)
		}
	})

	err = pc.SetLocalDescription(offer)
	require.NoError(t, err)

	err = pc.SetRemoteDescription(*answer)
	require.NoError(t, err)

	<-gatheringDone
	<-connected

	if publish {
		time.Sleep(200 * time.Millisecond)

		err := outgoingTrack1.WriteRTP(&rtp.Packet{
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
		require.NoError(t, err)

		err = outgoingTrack2.WriteRTP(&rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: 1123,
				Timestamp:      45343,
				SSRC:           563423,
			},
			Payload: []byte{0x01, 0x02, 0x03, 0x04},
		})
		require.NoError(t, err)

		time.Sleep(200 * time.Millisecond)
	}

	return &webRTCTestClient{
		pc:             pc,
		outgoingTrack1: outgoingTrack1,
		outgoingTrack2: outgoingTrack2,
		incomingTrack:  incomingTrack,
		closed:         closed,
	}
}

func (c *webRTCTestClient) close() {
	c.pc.Close()
	<-c.closed
}

func TestWebRTCRead(t *testing.T) {
	p, ok := newInstance("paths:\n" +
		"  all:\n")
	require.Equal(t, true, ok)
	defer p.Close()

	medi := &media.Media{
		Type: media.TypeVideo,
		Formats: []formats.Format{&formats.H264{
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

	c := newWebRTCTestClient(t, "http://localhost:8889/stream/whep", false)
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

	trak := <-c.incomingTrack

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

func TestWebRTCReadNotFound(t *testing.T) {
	p, ok := newInstance("paths:\n" +
		"  all:\n")
	require.Equal(t, true, ok)
	defer p.Close()

	iceServers := whipGetICEServers(t, "http://localhost:8889/stream/whep")

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: iceServers,
	})
	require.NoError(t, err)
	defer pc.Close()

	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	require.NoError(t, err)

	offer, err := pc.CreateOffer(nil)
	require.NoError(t, err)

	enc, err := json.Marshal(offer)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", "http://localhost:8889/stream/whep", bytes.NewReader(enc))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/sdp")

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestWebRTCPublish(t *testing.T) {
	p, ok := newInstance("paths:\n" +
		"  all:\n")
	require.Equal(t, true, ok)
	defer p.Close()

	s := newWebRTCTestClient(t, "http://localhost:8889/stream/whip", true)
	defer s.close()

	c := gortsplib.Client{
		OnDecodeError: func(err error) {
			panic(err)
		},
	}

	u, err := url.Parse("rtsp://127.0.0.1:8554/stream")
	require.NoError(t, err)

	err = c.Start(u.Scheme, u.Host)
	require.NoError(t, err)
	defer c.Close()

	medias, baseURL, _, err := c.Describe(u)
	require.NoError(t, err)

	var forma *formats.VP8
	medi := medias.FindFormat(&forma)

	_, err = c.Setup(medi, baseURL, 0, 0)
	require.NoError(t, err)

	received := make(chan struct{})

	c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
		require.Equal(t, []byte{0x05, 0x06, 0x07, 0x08}, pkt.Payload)
		close(received)
	})

	_, err = c.Play(nil)
	require.NoError(t, err)

	err = s.outgoingTrack1.WriteRTP(&rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         true,
			PayloadType:    96,
			SequenceNumber: 124,
			Timestamp:      45343,
			SSRC:           563423,
		},
		Payload: []byte{0x05, 0x06, 0x07, 0x08},
	})
	require.NoError(t, err)

	<-received
}
