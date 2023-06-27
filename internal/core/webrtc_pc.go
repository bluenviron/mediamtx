package core

import (
	"strconv"
	"sync"

	"github.com/pion/ice/v2"
	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v3"

	"github.com/bluenviron/mediamtx/internal/logger"
)

var videoCodecs = map[string][]webrtc.RTPCodecParameters{
	"av1": {{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeAV1,
			ClockRate: 90000,
		},
		PayloadType: 96,
	}},
	"vp9": {
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeVP9,
				ClockRate:   90000,
				SDPFmtpLine: "profile-id=0",
			},
			PayloadType: 96,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeVP9,
				ClockRate:   90000,
				SDPFmtpLine: "profile-id=1",
			},
			PayloadType: 96,
		},
	},
	"vp8": {{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeVP8,
			ClockRate: 90000,
		},
		PayloadType: 96,
	}},
	"h264": {{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f",
		},
		PayloadType: 96,
	}},
}

var audioCodecs = map[string][]webrtc.RTPCodecParameters{
	"opus": {{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeOpus,
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "minptime=10;useinbandfec=1",
		},
		PayloadType: 111,
	}},
	"g722": {{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeG722,
			ClockRate: 8000,
		},
		PayloadType: 9,
	}},
	"pcmu": {{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMU,
			ClockRate: 8000,
		},
		PayloadType: 0,
	}},
	"pcma": {{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMA,
			ClockRate: 8000,
		},
		PayloadType: 8,
	}},
}

type peerConnection struct {
	*webrtc.PeerConnection
	stateChangeMutex   sync.Mutex
	localCandidateRecv chan *webrtc.ICECandidateInit
	connected          chan struct{}
	disconnected       chan struct{}
	closed             chan struct{}
	gatheringDone      chan struct{}
}

func newPeerConnection(
	videoCodec string,
	audioCodec string,
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

	if videoCodec != "" || audioCodec != "" {
		codec, ok := videoCodecs[videoCodec]
		if ok {
			for _, params := range codec {
				err := mediaEngine.RegisterCodec(params, webrtc.RTPCodecTypeVideo)
				if err != nil {
					return nil, err
				}
			}
		}

		codec, ok = audioCodecs[audioCodec]
		if ok {
			for _, params := range codec {
				err := mediaEngine.RegisterCodec(params, webrtc.RTPCodecTypeAudio)
				if err != nil {
					return nil, err
				}
			}
		}
	} else { // register all codecs
		for _, codec := range videoCodecs {
			for _, params := range codec {
				err := mediaEngine.RegisterCodec(params, webrtc.RTPCodecTypeVideo)
				if err != nil {
					return nil, err
				}
			}
		}

		for _, codec := range audioCodecs {
			for _, params := range codec {
				err := mediaEngine.RegisterCodec(params, webrtc.RTPCodecTypeAudio)
				if err != nil {
					return nil, err
				}
			}
		}
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
		gatheringDone:      make(chan struct{}),
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
				co.localCandidate(), co.remoteCandidate())

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
		} else {
			close(co.gatheringDone)
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
