// Package webrtc contains WebRTC utilities.
package webrtc

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"

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

// skip ConfigureRTCPReports
func registerInterceptors(mediaEngine *webrtc.MediaEngine, interceptorRegistry *interceptor.Registry) error {
	if err := webrtc.ConfigureNack(mediaEngine, interceptorRegistry); err != nil {
		return err
	}

	if err := webrtc.ConfigureSimulcastExtensionHeaders(mediaEngine); err != nil {
		return err
	}

	return webrtc.ConfigureTWCCSender(mediaEngine, interceptorRegistry)
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
		return fmt.Errorf("no valid tracks found")
	}

	return nil
}

type trackRecvPair struct {
	track    *webrtc.TrackRemote
	receiver *webrtc.RTPReceiver
}

// PeerConnection is a wrapper around webrtc.PeerConnection.
type PeerConnection struct {
	LocalRandomUDP        bool
	ICEUDPMux             ice.UDPMux
	ICETCPMux             ice.TCPMux
	ICEServers            []webrtc.ICEServer
	IPsFromInterfaces     bool
	IPsFromInterfacesList []string
	AdditionalHosts       []string
	HandshakeTimeout      conf.Duration
	TrackGatherTimeout    conf.Duration
	STUNGatherTimeout     conf.Duration
	Publish               bool
	OutgoingTracks        []*OutgoingTrack
	UseAbsoluteTimestamp  bool
	Log                   logger.Writer

	wr                *webrtc.PeerConnection
	stateChangeMutex  sync.Mutex
	newLocalCandidate chan *webrtc.ICECandidateInit
	connected         chan struct{}
	failed            chan struct{}
	closed            chan struct{}
	gatheringDone     chan struct{}
	incomingTrack     chan trackRecvPair
	ctx               context.Context
	ctxCancel         context.CancelFunc
	incomingTracks    []*IncomingTrack
}

// Start starts the peer connection.
func (co *PeerConnection) Start() error {
	settingsEngine := webrtc.SettingEngine{}

	settingsEngine.SetIncludeLoopbackCandidate(true)

	settingsEngine.SetInterfaceFilter(func(iface string) bool {
		return co.IPsFromInterfaces && (len(co.IPsFromInterfacesList) == 0 ||
			stringInSlice(iface, co.IPsFromInterfacesList))
	})

	settingsEngine.SetAdditionalHosts(co.AdditionalHosts)

	// always enable all networks since we might be the client of a remote TCP listener
	settingsEngine.SetNetworkTypes([]webrtc.NetworkType{
		webrtc.NetworkTypeUDP4,
		webrtc.NetworkTypeTCP4,
	})

	if co.ICEUDPMux != nil {
		settingsEngine.SetICEUDPMux(co.ICEUDPMux)
	}

	if co.ICETCPMux != nil {
		settingsEngine.SetICETCPMux(co.ICETCPMux)
	}

	if co.LocalRandomUDP {
		settingsEngine.SetLocalRandomUDP(true)
	}

	settingsEngine.SetSTUNGatherTimeout(time.Duration(co.STUNGatherTimeout))

	mediaEngine := &webrtc.MediaEngine{}

	if co.Publish {
		videoSetupped := false
		audioSetupped := false
		for _, tr := range co.OutgoingTracks {
			if tr.isVideo() {
				videoSetupped = true
			} else {
				audioSetupped = true
			}
		}

		// When audio is not used, a track has to be present anyway,
		// otherwise video is not displayed on Firefox and Chrome.
		if !audioSetupped {
			co.OutgoingTracks = append(co.OutgoingTracks, &OutgoingTrack{
				Caps: webrtc.RTPCodecCapability{
					MimeType:  webrtc.MimeTypePCMU,
					ClockRate: 8000,
				},
			})
		}

		for i, tr := range co.OutgoingTracks {
			var codecType webrtc.RTPCodecType
			if tr.isVideo() {
				codecType = webrtc.RTPCodecTypeVideo
			} else {
				codecType = webrtc.RTPCodecTypeAudio
			}

			err := mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
				RTPCodecCapability: tr.Caps,
				PayloadType:        webrtc.PayloadType(96 + i),
			}, codecType)
			if err != nil {
				return err
			}
		}

		// When video is not used, a track must not be added but a codec has to present.
		// Otherwise audio is muted on Firefox and Chrome.
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

	err := registerInterceptors(mediaEngine, interceptorRegistry)
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
	co.failed = make(chan struct{})
	co.closed = make(chan struct{})
	co.gatheringDone = make(chan struct{})
	co.incomingTrack = make(chan trackRecvPair)

	co.ctx, co.ctxCancel = context.WithCancel(context.Background())

	if co.Publish {
		for _, tr := range co.OutgoingTracks {
			err = tr.setup(co)
			if err != nil {
				co.wr.GracefulClose() //nolint:errcheck
				return err
			}
		}
	} else {
		_, err = co.wr.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionRecvonly,
		})
		if err != nil {
			co.wr.GracefulClose() //nolint:errcheck
			return err
		}

		_, err = co.wr.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionRecvonly,
		})
		if err != nil {
			co.wr.GracefulClose() //nolint:errcheck
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
		case <-co.closed:
			return
		default:
		}

		co.Log.Log(logger.Debug, "peer connection state: "+state.String())

		switch state {
		case webrtc.PeerConnectionStateConnected:
			// PeerConnectionStateConnected can arrive twice, since state can
			// switch from "disconnected" to "connected".
			// contrarily, we're interested into emitting "connected" once.
			select {
			case <-co.connected:
				return
			default:
			}

			co.Log.Log(logger.Info, "peer connection established, local candidate: %v, remote candidate: %v",
				co.LocalCandidate(), co.RemoteCandidate())

			close(co.connected)

		case webrtc.PeerConnectionStateFailed:
			close(co.failed)

		case webrtc.PeerConnectionStateClosed:
			// "closed" can arrive before "failed" and without
			// the Close() method being called at all.
			// It happens when the other peer sends a termination
			// message like a DTLS CloseNotify.
			select {
			case <-co.failed:
			default:
				close(co.failed)
			}

			close(co.closed)
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
	for _, track := range co.incomingTracks {
		track.close()
	}
	for _, track := range co.OutgoingTracks {
		track.close()
	}

	co.ctxCancel()
	co.wr.GracefulClose() //nolint:errcheck

	// even if GracefulClose() should wait for any goroutine to return,
	// we have to wait for OnConnectionStateChange to return anyway,
	// since it is executed in an uncontrolled goroutine.
	// https://github.com/pion/webrtc/blob/4742d1fd54abbc3f81c3b56013654574ba7254f3/peerconnection.go#L509
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
func (co *PeerConnection) GatherIncomingTracks(ctx context.Context) error {
	var sdp sdp.SessionDescription
	sdp.Unmarshal([]byte(co.wr.RemoteDescription().SDP)) //nolint:errcheck

	maxTrackCount := len(sdp.MediaDescriptions)

	t := time.NewTimer(time.Duration(co.TrackGatherTimeout))
	defer t.Stop()

	for {
		select {
		case <-t.C:
			if len(co.incomingTracks) != 0 {
				return nil
			}
			return fmt.Errorf("deadline exceeded while waiting tracks")

		case pair := <-co.incomingTrack:
			t := &IncomingTrack{
				useAbsoluteTimestamp: co.UseAbsoluteTimestamp,
				track:                pair.track,
				receiver:             pair.receiver,
				writeRTCP:            co.wr.WriteRTCP,
				log:                  co.Log,
			}
			t.initialize()
			co.incomingTracks = append(co.incomingTracks, t)

			if len(co.incomingTracks) >= maxTrackCount {
				return nil
			}

		case <-co.Failed():
			return fmt.Errorf("peer connection closed")

		case <-ctx.Done():
			return fmt.Errorf("terminated")
		}
	}
}

// Connected returns when connected.
func (co *PeerConnection) Connected() <-chan struct{} {
	return co.connected
}

// Failed returns when failed.
func (co *PeerConnection) Failed() <-chan struct{} {
	return co.failed
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

// IncomingTracks returns incoming tracks.
func (co *PeerConnection) IncomingTracks() []*IncomingTrack {
	return co.incomingTracks
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
