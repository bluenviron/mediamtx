package webrtc

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/webrtc/v3"

	"github.com/bluenviron/mediamtx/internal/logger"
)

const (
	webrtcHandshakeTimeout   = 10 * time.Second
	webrtcTrackGatherTimeout = 2 * time.Second
	webrtcStreamID           = "mediamtx"
)

type trackRecvPair struct {
	track    *webrtc.TrackRemote
	receiver *webrtc.RTPReceiver
}

// PeerConnection is a wrapper around webrtc.PeerConnection.
type PeerConnection struct {
	ICEServers []webrtc.ICEServer
	API        *webrtc.API
	Publish    bool
	Log        logger.Writer

	wr                *webrtc.PeerConnection
	stateChangeMutex  sync.Mutex
	newLocalCandidate chan *webrtc.ICECandidateInit
	connected         chan struct{}
	disconnected      chan struct{}
	closed            chan struct{}
	gatheringDone     chan struct{}
	incomingTrack     chan trackRecvPair
}

// Start starts the peer connection.
func (co *PeerConnection) Start() error {
	configuration := webrtc.Configuration{
		ICEServers: co.ICEServers,
	}

	var err error
	co.wr, err = co.API.NewPeerConnection(configuration)
	if err != nil {
		return err
	}

	co.newLocalCandidate = make(chan *webrtc.ICECandidateInit)
	co.connected = make(chan struct{})
	co.disconnected = make(chan struct{})
	co.closed = make(chan struct{})
	co.gatheringDone = make(chan struct{})
	co.incomingTrack = make(chan trackRecvPair)

	if !co.Publish {
		_, err = co.wr.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RtpTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionRecvonly,
		})
		if err != nil {
			co.wr.Close() //nolint:errcheck
			return err
		}

		_, err = co.wr.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RtpTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionRecvonly,
		})
		if err != nil {
			co.wr.Close() //nolint:errcheck
			return err
		}

		co.wr.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
			select {
			case co.incomingTrack <- trackRecvPair{track, receiver}:
			case <-co.closed:
			}
		})
	}

	co.wr.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		co.stateChangeMutex.Lock()
		defer co.stateChangeMutex.Unlock()

		select {
		case <-co.closed:
			return
		default:
		}

		co.Log.Log(logger.Debug, "peer connection state: "+state.String())

		switch state {
		case webrtc.PeerConnectionStateConnected:
			co.Log.Log(logger.Info, "peer connection established, local candidate: %v, remote candidate: %v",
				co.LocalCandidate(), co.RemoteCandidate())

			close(co.connected)

		case webrtc.PeerConnectionStateDisconnected:
			close(co.disconnected)

		case webrtc.PeerConnectionStateClosed:
			close(co.closed)
		}
	})

	co.wr.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i != nil {
			v := i.ToJSON()
			select {
			case co.newLocalCandidate <- &v:
			case <-co.connected:
			case <-co.closed:
			}
		} else {
			close(co.gatheringDone)
		}
	})

	return nil
}

// Close closes the connection.
func (co *PeerConnection) Close() {
	co.wr.Close() //nolint:errcheck
	<-co.closed
}

// CreatePartialOffer creates a partial offer.
func (co *PeerConnection) CreatePartialOffer() (*webrtc.SessionDescription, error) {
	offer, err := co.wr.CreateOffer(nil)
	if err != nil {
		return nil, err
	}

	err = co.wr.SetLocalDescription(offer)
	if err != nil {
		return nil, err
	}

	return &offer, nil
}

// SetAnswer sets the answer.
func (co *PeerConnection) SetAnswer(answer *webrtc.SessionDescription) error {
	return co.wr.SetRemoteDescription(*answer)
}

// AddRemoteCandidate adds a remote candidate.
func (co *PeerConnection) AddRemoteCandidate(candidate webrtc.ICECandidateInit) error {
	return co.wr.AddICECandidate(candidate)
}

// CreateFullAnswer creates a full answer.
func (co *PeerConnection) CreateFullAnswer(
	ctx context.Context,
	offer *webrtc.SessionDescription,
) (*webrtc.SessionDescription, error) {
	err := co.wr.SetRemoteDescription(*offer)
	if err != nil {
		return nil, err
	}

	answer, err := co.wr.CreateAnswer(nil)
	if err != nil {
		return nil, err
	}

	err = co.wr.SetLocalDescription(answer)
	if err != nil {
		return nil, err
	}

	err = co.WaitGatheringDone(ctx)
	if err != nil {
		return nil, err
	}

	return co.wr.LocalDescription(), nil
}

// WaitGatheringDone waits until candidate gathering is complete.
func (co *PeerConnection) WaitGatheringDone(ctx context.Context) error {
	for {
		select {
		case <-co.NewLocalCandidate():
		case <-co.GatheringDone():
			return nil
		case <-ctx.Done():
			return fmt.Errorf("terminated")
		}
	}
}

// WaitUntilConnected waits until connection is established.
func (co *PeerConnection) WaitUntilConnected(
	ctx context.Context,
) error {
	t := time.NewTimer(webrtcHandshakeTimeout)
	defer t.Stop()

outer:
	for {
		select {
		case <-t.C:
			return fmt.Errorf("deadline exceeded while waiting connection")

		case <-co.connected:
			break outer

		case <-ctx.Done():
			return fmt.Errorf("terminated")
		}
	}

	return nil
}

// GatherIncomingTracks gathers incoming tracks.
func (co *PeerConnection) GatherIncomingTracks(
	ctx context.Context,
	maxCount int,
) ([]*IncomingTrack, error) {
	var tracks []*IncomingTrack

	t := time.NewTimer(webrtcTrackGatherTimeout)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			if maxCount == 0 && len(tracks) != 0 {
				return tracks, nil
			}
			return nil, fmt.Errorf("deadline exceeded while waiting tracks")

		case pair := <-co.incomingTrack:
			track, err := newIncomingTrack(pair.track, pair.receiver, co.wr.WriteRTCP, co.Log)
			if err != nil {
				return nil, err
			}
			tracks = append(tracks, track)

			if len(tracks) == maxCount || len(tracks) >= 2 {
				return tracks, nil
			}

		case <-co.Disconnected():
			return nil, fmt.Errorf("peer connection closed")

		case <-ctx.Done():
			return nil, fmt.Errorf("terminated")
		}
	}
}

// SetupOutgoingTracks setups outgoing tracks.
func (co *PeerConnection) SetupOutgoingTracks(
	videoTrack format.Format,
	audioTrack format.Format,
) ([]*OutgoingTrack, error) {
	var tracks []*OutgoingTrack

	for _, forma := range []format.Format{videoTrack, audioTrack} {
		if forma != nil {
			track, err := newOutgoingTrack(forma, co.wr.AddTrack)
			if err != nil {
				return nil, err
			}

			tracks = append(tracks, track)
		}
	}

	return tracks, nil
}

// Connected returns when connected.
func (co *PeerConnection) Connected() <-chan struct{} {
	return co.connected
}

// Disconnected returns when disconnected.
func (co *PeerConnection) Disconnected() <-chan struct{} {
	return co.disconnected
}

// NewLocalCandidate returns when there's a new local candidate.
func (co *PeerConnection) NewLocalCandidate() <-chan *webrtc.ICECandidateInit {
	return co.newLocalCandidate
}

// GatheringDone returns when candidate gathering is complete.
func (co *PeerConnection) GatheringDone() <-chan struct{} {
	return co.gatheringDone
}

// LocalCandidate returns the local candidate.
func (co *PeerConnection) LocalCandidate() string {
	var cid string
	for _, stats := range co.wr.GetStats() {
		if tstats, ok := stats.(webrtc.ICECandidatePairStats); ok && tstats.Nominated {
			cid = tstats.LocalCandidateID
			break
		}
	}

	if cid != "" {
		for _, stats := range co.wr.GetStats() {
			if tstats, ok := stats.(webrtc.ICECandidateStats); ok && tstats.ID == cid {
				return tstats.CandidateType.String() + "/" + tstats.Protocol + "/" +
					tstats.IP + "/" + strconv.FormatInt(int64(tstats.Port), 10)
			}
		}
	}

	return ""
}

// RemoteCandidate returns the remote candidate.
func (co *PeerConnection) RemoteCandidate() string {
	var cid string
	for _, stats := range co.wr.GetStats() {
		if tstats, ok := stats.(webrtc.ICECandidatePairStats); ok && tstats.Nominated {
			cid = tstats.RemoteCandidateID
			break
		}
	}

	if cid != "" {
		for _, stats := range co.wr.GetStats() {
			if tstats, ok := stats.(webrtc.ICECandidateStats); ok && tstats.ID == cid {
				return tstats.CandidateType.String() + "/" + tstats.Protocol + "/" +
					tstats.IP + "/" + strconv.FormatInt(int64(tstats.Port), 10)
			}
		}
	}

	return ""
}

// BytesReceived returns received bytes.
func (co *PeerConnection) BytesReceived() uint64 {
	for _, stats := range co.wr.GetStats() {
		if tstats, ok := stats.(webrtc.TransportStats); ok {
			if tstats.ID == "iceTransport" {
				return tstats.BytesReceived
			}
		}
	}
	return 0
}

// BytesSent returns sent bytes.
func (co *PeerConnection) BytesSent() uint64 {
	for _, stats := range co.wr.GetStats() {
		if tstats, ok := stats.(webrtc.TransportStats); ok {
			if tstats.ID == "iceTransport" {
				return tstats.BytesSent
			}
		}
	}
	return 0
}
