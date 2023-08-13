// Package webrtcpc contains a WebRTC peer connection wrapper.
package webrtcpc

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/pion/webrtc/v3"

	"github.com/bluenviron/mediamtx/internal/logger"
)

// PeerConnection is a wrapper around webrtc.PeerConnection.
type PeerConnection struct {
	*webrtc.PeerConnection
	stateChangeMutex  sync.Mutex
	newLocalCandidate chan *webrtc.ICECandidateInit
	connected         chan struct{}
	disconnected      chan struct{}
	closed            chan struct{}
	gatheringDone     chan struct{}
}

// New allocates a PeerConnection.
func New(
	iceServers []webrtc.ICEServer,
	api *webrtc.API,
	log logger.Writer,
) (*PeerConnection, error) {
	configuration := webrtc.Configuration{ICEServers: iceServers}

	pc, err := api.NewPeerConnection(configuration)
	if err != nil {
		return nil, err
	}

	co := &PeerConnection{
		PeerConnection:    pc,
		newLocalCandidate: make(chan *webrtc.ICECandidateInit),
		connected:         make(chan struct{}),
		disconnected:      make(chan struct{}),
		closed:            make(chan struct{}),
		gatheringDone:     make(chan struct{}),
	}

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		co.stateChangeMutex.Lock()
		defer co.stateChangeMutex.Unlock()

		select {
		case <-co.closed:
			return
		default:
		}

		log.Log(logger.Debug, "peer connection state: "+state.String())

		switch state {
		case webrtc.PeerConnectionStateConnected:
			log.Log(logger.Info, "peer connection established, local candidate: %v, remote candidate: %v",
				co.LocalCandidate(), co.RemoteCandidate())

			close(co.connected)

		case webrtc.PeerConnectionStateDisconnected:
			close(co.disconnected)

		case webrtc.PeerConnectionStateClosed:
			close(co.closed)
		}
	})

	pc.OnICECandidate(func(i *webrtc.ICECandidate) {
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

	return co, nil
}

// Close closes the connection.
func (co *PeerConnection) Close() {
	co.PeerConnection.Close() //nolint:errcheck
	<-co.closed
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

// LocalCandidate returns the local candidate.
func (co *PeerConnection) LocalCandidate() string {
	var cid string
	for _, stats := range co.GetStats() {
		if tstats, ok := stats.(webrtc.ICECandidatePairStats); ok && tstats.Nominated {
			cid = tstats.LocalCandidateID
			break
		}
	}

	if cid != "" {
		for _, stats := range co.GetStats() {
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
	for _, stats := range co.GetStats() {
		if tstats, ok := stats.(webrtc.ICECandidatePairStats); ok && tstats.Nominated {
			cid = tstats.RemoteCandidateID
			break
		}
	}

	if cid != "" {
		for _, stats := range co.GetStats() {
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
	for _, stats := range co.GetStats() {
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
	for _, stats := range co.GetStats() {
		if tstats, ok := stats.(webrtc.TransportStats); ok {
			if tstats.ID == "iceTransport" {
				return tstats.BytesSent
			}
		}
	}
	return 0
}
