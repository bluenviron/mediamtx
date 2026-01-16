// Package webrtc contains WebRTC utilities.
package webrtc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

func interfaceIPs(interfaceList []string) ([]string, error) {
	intfs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var ips []string

	for _, intf := range intfs {
		if len(interfaceList) == 0 || slices.Contains(interfaceList, intf.Name) {
			var addrs []net.Addr
			addrs, err = intf.Addrs()
			if err == nil {
				for _, addr := range addrs {
					var ip net.IP

					switch v := addr.(type) {
					case *net.IPNet:
						ip = v.IP
					case *net.IPAddr:
						ip = v.IP
					}

					if ip != nil {
						ips = append(ips, ip.String())
					}
				}
			}
		}
	}

	return ips, nil
}

// * skip ConfigureRTCPReports
// * add statsInterceptor
func registerInterceptors(
	mediaEngine *webrtc.MediaEngine,
	interceptorRegistry *interceptor.Registry,
	onStatsInterceptor func(s *statsInterceptor),
) error {
	err := webrtc.ConfigureNack(mediaEngine, interceptorRegistry)
	if err != nil {
		return err
	}

	err = webrtc.ConfigureSimulcastExtensionHeaders(mediaEngine)
	if err != nil {
		return err
	}

	err = webrtc.ConfigureTWCCSender(mediaEngine, interceptorRegistry)
	if err != nil {
		return err
	}

	interceptorRegistry.Add(&statsInterceptorFactory{
		onCreate: onStatsInterceptor,
	})

	return nil
}

func candidateLabel(c *webrtc.ICECandidate) string {
	return c.Typ.String() + "/" + c.Protocol.String() + "/" +
		c.Address + "/" + strconv.FormatInt(int64(c.Port), 10)
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
	UDPReadBufferSize     uint
	LocalRandomUDP        bool
	ICEUDPMux             ice.UDPMux
	ICETCPMux             *TCPMuxWrapper
	ICEServers            []webrtc.ICEServer
	IPsFromInterfaces     bool
	IPsFromInterfacesList []string
	AdditionalHosts       []string
	HandshakeTimeout      conf.Duration
	TrackGatherTimeout    conf.Duration
	STUNGatherTimeout     conf.Duration
	Publish               bool
	OutgoingTracks        []*OutgoingTrack
	OutgoingDataChannels  []*OutgoingDataChannel
	Log                   logger.Writer

	wr               *webrtc.PeerConnection
	ctx              context.Context
	ctxCancel        context.CancelFunc
	incomingTracks   []*IncomingTrack
	startedReading   *int64
	statsInterceptor *statsInterceptor

	newLocalCandidate chan *webrtc.ICECandidateInit
	incomingTrack     chan trackRecvPair
	connected         chan struct{}
	failed            chan struct{}
	closed            chan struct{}
	gatheringDone     chan struct{}
	done              chan struct{}
	chStartReading    chan struct{}
}

// Start starts the peer connection.
func (co *PeerConnection) Start() error {
	settingsEngine := webrtc.SettingEngine{}

	settingsEngine.SetIncludeLoopbackCandidate(true)

	// always enable TCP since we might be the client of a remote TCP listener
	networkTypes := []webrtc.NetworkType{
		webrtc.NetworkTypeTCP4,
		webrtc.NetworkTypeTCP6,
	}

	if co.LocalRandomUDP || co.ICEUDPMux != nil || len(co.ICEServers) != 0 {
		networkTypes = append(networkTypes, webrtc.NetworkTypeUDP4, webrtc.NetworkTypeUDP6)
	}

	settingsEngine.SetNetworkTypes(networkTypes)

	if co.ICEUDPMux != nil {
		settingsEngine.SetICEUDPMux(co.ICEUDPMux)
	}

	if co.ICETCPMux != nil {
		settingsEngine.SetICETCPMux(co.ICETCPMux.Mux)
	}

	settingsEngine.SetSTUNGatherTimeout(time.Duration(co.STUNGatherTimeout))

	webrtcNet := &webrtcNet{
		udpReadBufferSize: int(co.UDPReadBufferSize),
	}
	err := webrtcNet.initialize()
	if err != nil {
		return err
	}
	settingsEngine.SetNet(webrtcNet)

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

			err = mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
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
			err = mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
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
			err = mediaEngine.RegisterCodec(codec, webrtc.RTPCodecTypeVideo)
			if err != nil {
				return err
			}
		}

		for _, codec := range incomingAudioCodecs {
			err = mediaEngine.RegisterCodec(codec, webrtc.RTPCodecTypeAudio)
			if err != nil {
				return err
			}
		}
	}

	interceptorRegistry := &interceptor.Registry{}

	err = registerInterceptors(
		mediaEngine,
		interceptorRegistry,
		func(s *statsInterceptor) {
			co.statsInterceptor = s
		},
	)
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

	co.ctx, co.ctxCancel = context.WithCancel(context.Background())

	co.startedReading = new(int64)

	co.newLocalCandidate = make(chan *webrtc.ICECandidateInit)
	co.connected = make(chan struct{})
	co.failed = make(chan struct{})
	co.closed = make(chan struct{})
	co.gatheringDone = make(chan struct{})
	co.incomingTrack = make(chan trackRecvPair)
	co.done = make(chan struct{})
	co.chStartReading = make(chan struct{})

	if co.Publish {
		for _, tr := range co.OutgoingTracks {
			err = tr.setup(co)
			if err != nil {
				co.wr.GracefulClose() //nolint:errcheck
				return err
			}
		}

		for _, dc := range co.OutgoingDataChannels {
			err = dc.setup(co)
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

	var stateChangeMutex sync.Mutex

	co.wr.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		stateChangeMutex.Lock()
		defer stateChangeMutex.Unlock()

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

	go co.run()

	return nil
}

// Close closes the connection.
func (co *PeerConnection) Close() {
	co.ctxCancel()
	<-co.done
}

func (co *PeerConnection) run() {
	defer close(co.done)

	defer func() {
		for _, track := range co.incomingTracks {
			track.close()
		}
		for _, track := range co.OutgoingTracks {
			track.close()
		}

		co.wr.GracefulClose() //nolint:errcheck

		// even if GracefulClose() should wait for any goroutine to return,
		// we have to wait for OnConnectionStateChange to return anyway,
		// since it is executed in an uncontrolled goroutine.
		// https://github.com/pion/webrtc/blob/4742d1fd54abbc3f81c3b56013654574ba7254f3/peerconnection.go#L509
		<-co.closed
	}()

	for {
		select {
		case <-co.chStartReading:
			for _, track := range co.incomingTracks {
				track.start()
			}
			atomic.StoreInt64(co.startedReading, 1)

		case <-co.ctx.Done():
			return
		}
	}
}

func (co *PeerConnection) removeUnwantedCandidates(firstMedia *sdp.MediaDescription) error {
	var allowedIPs []string
	if co.IPsFromInterfaces {
		var err error
		allowedIPs, err = interfaceIPs(co.IPsFromInterfacesList)
		if err != nil {
			return err
		}
	}

	var newAttributes []sdp.Attribute //nolint:prealloc

	for _, attr := range firstMedia.Attributes {
		if attr.Key == "candidate" {
			parts := strings.Split(attr.Value, " ")

			// hide random UDP candidates
			if !co.LocalRandomUDP && co.ICEUDPMux == nil && parts[2] == "udp" && parts[7] == "host" {
				continue
			}

			// hide disallowed IPs
			if parts[7] == "host" && !slices.Contains(allowedIPs, parts[4]) {
				continue
			}
		}

		newAttributes = append(newAttributes, attr)
	}

	firstMedia.Attributes = newAttributes

	return nil
}

func (co *PeerConnection) addAdditionalCandidates(firstMedia *sdp.MediaDescription) error {
	i := 0
	for _, attr := range firstMedia.Attributes {
		if attr.Key == "end-of-candidates" {
			break
		}
		i++
	}

	for _, host := range co.AdditionalHosts {
		var ips []string
		if net.ParseIP(host) != nil {
			ips = []string{host}
		} else {
			tmp, err := net.LookupIP(host)
			if err != nil {
				return err
			}

			ips = make([]string, len(tmp))
			for i, e := range tmp {
				ips[i] = e.String()
			}
		}

		for _, ip := range ips {
			newAttrs := append([]sdp.Attribute(nil), firstMedia.Attributes[:i]...)

			if co.ICEUDPMux != nil {
				port := strconv.FormatInt(int64(co.ICEUDPMux.GetListenAddresses()[0].(*net.UDPAddr).Port), 10)

				tmp, err := randUint32()
				if err != nil {
					return err
				}
				id := strconv.FormatInt(int64(tmp), 10)

				newAttrs = append(newAttrs, sdp.Attribute{
					Key:   "candidate",
					Value: id + " 1 udp 2130706431 " + ip + " " + port + " typ host",
				})
				newAttrs = append(newAttrs, sdp.Attribute{
					Key:   "candidate",
					Value: id + " 2 udp 2130706431 " + ip + " " + port + " typ host",
				})
			}

			if co.ICETCPMux != nil {
				port := strconv.FormatInt(int64(co.ICETCPMux.Ln.Addr().(*net.TCPAddr).Port), 10)

				tmp, err := randUint32()
				if err != nil {
					return err
				}
				id := strconv.FormatInt(int64(tmp), 10)

				newAttrs = append(newAttrs, sdp.Attribute{
					Key:   "candidate",
					Value: id + " 1 tcp 1671430143 " + ip + " " + port + " typ host tcptype passive",
				})
				newAttrs = append(newAttrs, sdp.Attribute{
					Key:   "candidate",
					Value: id + " 2 tcp 1671430143 " + ip + " " + port + " typ host tcptype passive",
				})
			}

			newAttrs = append(newAttrs, firstMedia.Attributes[i:]...)
			firstMedia.Attributes = newAttrs
		}
	}

	return nil
}

func (co *PeerConnection) filterLocalDescription(desc *webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	var psdp sdp.SessionDescription
	psdp.Unmarshal([]byte(desc.SDP)) //nolint:errcheck

	firstMedia := psdp.MediaDescriptions[0]

	err := co.removeUnwantedCandidates(firstMedia)
	if err != nil {
		return nil, err
	}

	err = co.addAdditionalCandidates(firstMedia)
	if err != nil {
		return nil, err
	}

	out, _ := psdp.Marshal()
	desc.SDP = string(out)

	return desc, nil
}

// CreatePartialOffer creates a partial offer.
func (co *PeerConnection) CreatePartialOffer() (*webrtc.SessionDescription, error) {
	tmp, err := co.wr.CreateOffer(nil)
	if err != nil {
		return nil, err
	}
	offer := &tmp

	err = co.wr.SetLocalDescription(*offer)
	if err != nil {
		return nil, err
	}

	offer, err = co.filterLocalDescription(offer)
	if err != nil {
		return nil, err
	}

	return offer, nil
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
func (co *PeerConnection) CreateFullAnswer(offer *webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	err := co.wr.SetRemoteDescription(*offer)
	if err != nil {
		return nil, err
	}

	tmp, err := co.wr.CreateAnswer(nil)
	if err != nil {
		if errors.Is(err, webrtc.ErrSenderWithNoCodecs) {
			return nil, fmt.Errorf("codecs not supported by client")
		}
		return nil, err
	}
	answer := &tmp

	err = co.wr.SetLocalDescription(*answer)
	if err != nil {
		return nil, err
	}

	err = co.waitGatheringDone()
	if err != nil {
		return nil, err
	}

	answer = co.wr.LocalDescription()

	answer, err = co.filterLocalDescription(answer)
	if err != nil {
		return nil, err
	}

	return answer, nil
}

func (co *PeerConnection) waitGatheringDone() error {
	for {
		select {
		case <-co.NewLocalCandidate():
		case <-co.GatheringDone():
			return nil
		case <-co.ctx.Done():
			return fmt.Errorf("terminated")
		}
	}
}

// WaitUntilConnected waits until connection is established.
func (co *PeerConnection) WaitUntilConnected() error {
	t := time.NewTimer(time.Duration(co.HandshakeTimeout))
	defer t.Stop()

outer:
	for {
		select {
		case <-t.C:
			return fmt.Errorf("deadline exceeded while waiting connection")

		case <-co.connected:
			break outer

		case <-co.ctx.Done():
			return fmt.Errorf("terminated")
		}
	}

	return nil
}

// GatherIncomingTracks gathers incoming tracks.
func (co *PeerConnection) GatherIncomingTracks() error {
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
				track:     pair.track,
				receiver:  pair.receiver,
				writeRTCP: co.wr.WriteRTCP,
				log:       co.Log,
			}
			t.initialize()
			co.incomingTracks = append(co.incomingTracks, t)

			if len(co.incomingTracks) >= maxTrackCount {
				return nil
			}

		case <-co.Failed():
			return fmt.Errorf("peer connection closed")

		case <-co.ctx.Done():
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

// IncomingTracks returns incoming tracks.
func (co *PeerConnection) IncomingTracks() []*IncomingTrack {
	return co.incomingTracks
}

// StartReading starts reading incoming tracks.
func (co *PeerConnection) StartReading() {
	select {
	case co.chStartReading <- struct{}{}:
	case <-co.ctx.Done():
	}
}

// LocalCandidate returns the local candidate.
func (co *PeerConnection) LocalCandidate() string {
	receivers := co.wr.GetReceivers()
	if len(receivers) < 1 {
		return ""
	}

	cp, err := receivers[0].Transport().ICETransport().GetSelectedCandidatePair()
	if err != nil || cp == nil {
		return ""
	}

	return candidateLabel(cp.Local)
}

// RemoteCandidate returns the remote candidate.
func (co *PeerConnection) RemoteCandidate() string {
	receivers := co.wr.GetReceivers()
	if len(receivers) < 1 {
		return ""
	}

	cp, err := receivers[0].Transport().ICETransport().GetSelectedCandidatePair()
	if err != nil || cp == nil {
		return ""
	}

	return candidateLabel(cp.Remote)
}

func bytesStats(wr *webrtc.PeerConnection) (uint64, uint64) {
	for _, stats := range wr.GetStats() {
		if tstats, ok := stats.(webrtc.TransportStats); ok {
			if tstats.ID == "iceTransport" {
				return tstats.BytesReceived, tstats.BytesSent
			}
		}
	}
	return 0, 0
}

// Stats returns statistics.
func (co *PeerConnection) Stats() *Stats {
	bytesReceived, bytesSent := bytesStats(co.wr)

	v := float64(0)
	n := float64(0)
	packetsReceived := uint64(0)
	packetsSent := uint64(0)
	packetsLost := uint64(0)

	if atomic.LoadInt64(co.startedReading) == 1 {
		for _, tr := range co.incomingTracks {
			if recvStats := tr.rtpReceiver.Stats(); recvStats != nil {
				v += recvStats.Jitter
				n++
				packetsReceived += recvStats.TotalReceived
				packetsLost += recvStats.TotalLost
			}
		}
	}

	for _, tr := range co.OutgoingTracks {
		if sentStats := tr.rtcpSender.Stats(); sentStats != nil {
			packetsSent += sentStats.TotalSent
		}
	}

	var rtpPacketsJitter float64
	if n != 0 {
		rtpPacketsJitter = v / n
	} else {
		rtpPacketsJitter = 0
	}

	return &Stats{
		BytesReceived:       bytesReceived,
		BytesSent:           bytesSent,
		RTPPacketsReceived:  packetsReceived,
		RTPPacketsSent:      packetsSent,
		RTPPacketsLost:      packetsLost,
		RTPPacketsJitter:    rtpPacketsJitter,
		RTCPPacketsReceived: atomic.LoadUint64(co.statsInterceptor.rtcpPacketsReceived),
		RTCPPacketsSent:     atomic.LoadUint64(co.statsInterceptor.rtcpPacketsSent),
	}
}
