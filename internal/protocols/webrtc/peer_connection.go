// Package webrtc contains WebRTC utilities.
package webrtc

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/pion/ice/v2"
	"github.com/pion/interceptor"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
)

const (
	webrtcStreamID = "mediamtx"
)

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// TracksAreValid checks whether tracks in the SDP are valid
func TracksAreValid(medias []*sdp.MediaDescription) error {
	videoTrack := false
	audioTrack := false

	for _, media := range medias {
		switch media.MediaName.Media {
		case "video":
			if videoTrack {
				return fmt.Errorf("only a single video and a single audio track are supported")
			}
			videoTrack = true

		case "audio":
			if audioTrack {
				return fmt.Errorf("only a single video and a single audio track are supported")
			}
			audioTrack = true

		default:
			return fmt.Errorf("unsupported media '%s'", media.MediaName.Media)
		}
	}

	if !videoTrack && !audioTrack {
		return fmt.Errorf("no valid tracks count")
	}

	return nil
}

type trackRecvPair struct {
	track    *webrtc.TrackRemote
	receiver *webrtc.RTPReceiver
}

// PeerConnection is a wrapper around webrtc.PeerConnection.
type PeerConnection struct {
	ICEServers            []webrtc.ICEServer
	ICEUDPMux             ice.UDPMux
	ICETCPMux             ice.TCPMux
	HandshakeTimeout      conf.StringDuration
	TrackGatherTimeout    conf.StringDuration
	LocalRandomUDP        bool
	IPsFromInterfaces     bool
	IPsFromInterfacesList []string
	AdditionalHosts       []string
	Publish               bool
	OutgoingTracks        []*OutgoingTrack
	Log                   logger.Writer

	wr                *webrtc.PeerConnection
	stateChangeMutex  sync.Mutex
	newLocalCandidate chan *webrtc.ICECandidateInit
	connected         chan struct{}
	disconnected      chan struct{}
	done              chan struct{}
	gatheringDone     chan struct{}
	incomingTrack     chan trackRecvPair
	ctx               context.Context
	ctxCancel         context.CancelFunc
	incomingTracks    []*IncomingTrack
}

// Start starts the peer connection.
func (co *PeerConnection) Start() error {
	settingsEngine := webrtc.SettingEngine{}

	settingsEngine.SetInterfaceFilter(func(iface string) bool {
		return co.IPsFromInterfaces && (len(co.IPsFromInterfacesList) == 0 ||
			stringInSlice(iface, co.IPsFromInterfacesList))
	})

	settingsEngine.SetAdditionalHosts(co.AdditionalHosts)

	var networkTypes []webrtc.NetworkType

	// always enable UDP in order to support STUN/TURN
	networkTypes = append(networkTypes, webrtc.NetworkTypeUDP4)

	if co.ICEUDPMux != nil {
		settingsEngine.SetICEUDPMux(co.ICEUDPMux)
	}

	if co.ICETCPMux != nil {
		settingsEngine.SetICETCPMux(co.ICETCPMux)
		networkTypes = append(networkTypes, webrtc.NetworkTypeTCP4)
	}

	if co.LocalRandomUDP {
		settingsEngine.SetICEUDPRandom(true)
	}

	settingsEngine.SetNetworkTypes(networkTypes)

	mediaEngine := &webrtc.MediaEngine{}

	if co.Publish {
		videoSetupped := false
		audioSetupped := false

		for _, tr := range co.OutgoingTracks {
			var codecType webrtc.RTPCodecType
			if tr.isVideo() {
				codecType = webrtc.RTPCodecTypeVideo
				videoSetupped = true
			} else {
				codecType = webrtc.RTPCodecTypeAudio
				audioSetupped = true
			}

			err := mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
				RTPCodecCapability: tr.Caps,
				PayloadType:        96,
			}, codecType)
			if err != nil {
				return err
			}
		}

		// always register at least a video and a audio codec
		// otherwise handshake will fail or audio will be muted on some clients (like Firefox)
		if !videoSetupped {
			err := mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:  webrtc.MimeTypeVP8,
					ClockRate: 90000,
				},
				PayloadType: 96,
			}, webrtc.RTPCodecTypeVideo)
			if err != nil {
				return err
			}
		}
		if !audioSetupped {
			err := mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:  webrtc.MimeTypePCMU,
					ClockRate: 8000,
				},
				PayloadType: 0,
			}, webrtc.RTPCodecTypeAudio)
			if err != nil {
				return err
			}
		}
	} else {
		for _, codec := range incomingVideoCodecs {
			err := mediaEngine.RegisterCodec(codec, webrtc.RTPCodecTypeVideo)
			if err != nil {
				return err
			}
		}

		for _, codec := range incomingAudioCodecs {
			err := mediaEngine.RegisterCodec(codec, webrtc.RTPCodecTypeAudio)
			if err != nil {
				return err
			}
		}
	}

	interceptorRegistry := &interceptor.Registry{}

	err := webrtc.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry)
	if err != nil {
		return err
	}

	api := webrtc.NewAPI(
		webrtc.WithSettingEngine(settingsEngine),
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithInterceptorRegistry(interceptorRegistry))

	co.wr, err = api.NewPeerConnection(webrtc.Configuration{
		ICEServers: co.ICEServers,
	})
	if err != nil {
		return err
	}

	co.newLocalCandidate = make(chan *webrtc.ICECandidateInit)
	co.connected = make(chan struct{})
	co.disconnected = make(chan struct{})
	co.done = make(chan struct{})
	co.gatheringDone = make(chan struct{})
	co.incomingTrack = make(chan trackRecvPair)

	co.ctx, co.ctxCancel = context.WithCancel(context.Background())

	if co.Publish {
		for _, tr := range co.OutgoingTracks {
			err = tr.setup(co)
			if err != nil {
				co.wr.Close() //nolint:errcheck
				return err
			}
		}
	} else {
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
			case <-co.ctx.Done():
			}
		})
	}

	co.wr.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		co.stateChangeMutex.Lock()
		defer co.stateChangeMutex.Unlock()

		select {
		case <-co.done:
			return
		default:
		}

		co.Log.Log(logger.Debug, "peer connection state: "+state.String())

		switch state {
		case webrtc.PeerConnectionStateConnected:
			// for some reasons, PeerConnectionStateConnected can arrive twice.
			// https://github.com/bluenviron/mediamtx/issues/3813
			select {
			case <-co.connected:
				return
			default:
			}

			co.Log.Log(logger.Info, "peer connection established, local candidate: %v, remote candidate: %v",
				co.LocalCandidate(), co.RemoteCandidate())

			close(co.connected)

		case webrtc.PeerConnectionStateDisconnected:
			close(co.disconnected)

		case webrtc.PeerConnectionStateClosed:
			close(co.done)
		}
	})

	co.wr.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i != nil {
			v := i.ToJSON()
			select {
			case co.newLocalCandidate <- &v:
			case <-co.connected:
			case <-co.ctx.Done():
			}
		} else {
			close(co.gatheringDone)
		}
	})

	return nil
}

// Close closes the connection.
func (co *PeerConnection) Close() {
	co.ctxCancel()
	co.wr.Close() //nolint:errcheck
	<-co.done
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
func (co *PeerConnection) AddRemoteCandidate(candidate *webrtc.ICECandidateInit) error {
	return co.wr.AddICECandidate(*candidate)
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
		if errors.Is(err, webrtc.ErrSenderWithNoCodecs) {
			return nil, fmt.Errorf("codecs not supported by client")
		}
		return nil, err
	}

	err = co.wr.SetLocalDescription(answer)
	if err != nil {
		return nil, err
	}

	err = co.waitGatheringDone(ctx)
	if err != nil {
		return nil, err
	}

	return co.wr.LocalDescription(), nil
}

func (co *PeerConnection) waitGatheringDone(ctx context.Context) error {
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
	t := time.NewTimer(time.Duration(co.HandshakeTimeout))
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
func (co *PeerConnection) GatherIncomingTracks(ctx context.Context) ([]*IncomingTrack, error) {
	var sdp sdp.SessionDescription
	sdp.Unmarshal([]byte(co.wr.RemoteDescription().SDP)) //nolint:errcheck

	maxTrackCount := len(sdp.MediaDescriptions)

	t := time.NewTimer(time.Duration(co.TrackGatherTimeout))
	defer t.Stop()

	for {
		select {
		case <-t.C:
			if len(co.incomingTracks) != 0 {
				return co.incomingTracks, nil
			}
			return nil, fmt.Errorf("deadline exceeded while waiting tracks")

		case pair := <-co.incomingTrack:
			t := &IncomingTrack{
				track:     pair.track,
				receiver:  pair.receiver,
				writeRTCP: co.wr.WriteRTCP,
				log:       co.Log,
			}
			t.initialize()
			co.incomingTracks = append(co.incomingTracks, t)

			if len(co.incomingTracks) >= maxTrackCount {
				return co.incomingTracks, nil
			}

		case <-co.Disconnected():
			return nil, fmt.Errorf("peer connection closed")

		case <-ctx.Done():
			return nil, fmt.Errorf("terminated")
		}
	}
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

// StartReading starts reading all incoming tracks.
func (co *PeerConnection) StartReading() {
	for _, track := range co.incomingTracks {
		track.start()
	}
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
