package core

import (
	"strconv"
	"sync"

	"github.com/pion/ice/v2"
	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v3"

	"github.com/aler9/mediamtx/internal/logger"
)

type peerConnection struct {
	*webrtc.PeerConnection
	stateChangeMutex   sync.Mutex
	localCandidateRecv chan *webrtc.ICECandidateInit
	connected          chan struct{}
	disconnected       chan struct{}
	closed             chan struct{}
}

func newPeerConnection(
	iceServers []webrtc.ICEServer,
	iceHostNAT1To1IPs []string,
	iceUDPMux ice.UDPMux,
	iceTCPMux ice.TCPMux,
	log logger.Writer,
) (*peerConnection, error) {
	configuration := webrtc.Configuration{ICEServers: iceServers}
	settingsEngine := webrtc.SettingEngine{}

	if len(iceHostNAT1To1IPs) != 0 {
		settingsEngine.SetNAT1To1IPs(iceHostNAT1To1IPs, webrtc.ICECandidateTypeHost)
	}

	if iceUDPMux != nil {
		settingsEngine.SetICEUDPMux(iceUDPMux)
	}

	if iceTCPMux != nil {
		settingsEngine.SetICETCPMux(iceTCPMux)
		settingsEngine.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeTCP4})
	}

	mediaEngine := &webrtc.MediaEngine{}

	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return nil, err
	}

	err := mediaEngine.RegisterCodec(
		webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeAV1,
				ClockRate: 90000,
			},
			PayloadType: 105,
		},
		webrtc.RTPCodecTypeVideo)
	if err != nil {
		return nil, err
	}

	interceptorRegistry := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry); err != nil {
		return nil, err
	}

	api := webrtc.NewAPI(
		webrtc.WithSettingEngine(settingsEngine),
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithInterceptorRegistry(interceptorRegistry))

	pc, err := api.NewPeerConnection(configuration)
	if err != nil {
		return nil, err
	}

	co := &peerConnection{
		PeerConnection:     pc,
		localCandidateRecv: make(chan *webrtc.ICECandidateInit),
		connected:          make(chan struct{}),
		disconnected:       make(chan struct{}),
		closed:             make(chan struct{}),
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
			case co.localCandidateRecv <- &v:
			case <-co.connected:
			case <-co.closed:
			}
		}
	})

	return co, nil
}

func (co *peerConnection) close() {
	co.PeerConnection.Close()
	<-co.closed
}

func (co *peerConnection) localCandidate() string {
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

func (co *peerConnection) remoteCandidate() string {
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

func (co *peerConnection) bytesReceived() uint64 {
	for _, stats := range co.GetStats() {
		if tstats, ok := stats.(webrtc.TransportStats); ok {
			if tstats.ID == "iceTransport" {
				return tstats.BytesReceived
			}
		}
	}
	return 0
}

func (co *peerConnection) bytesSent() uint64 {
	for _, stats := range co.GetStats() {
		if tstats, ok := stats.(webrtc.TransportStats); ok {
			if tstats.ID == "iceTransport" {
				return tstats.BytesSent
			}
		}
	}
	return 0
}
