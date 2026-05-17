package webrtc

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/google/uuid"
	"github.com/pion/ice/v4"
	"github.com/pion/sdp/v3"
	pwebrtc "github.com/pion/webrtc/v4"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/hooks"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
	"github.com/bluenviron/mediamtx/internal/protocols/webrtc"
	"github.com/bluenviron/mediamtx/internal/protocols/whip"
	"github.com/bluenviron/mediamtx/internal/stream"
)

func whipOffer(body []byte) *pwebrtc.SessionDescription {
	return &pwebrtc.SessionDescription{
		Type: pwebrtc.SDPTypeOffer,
		SDP:  string(body),
	}
}

func parseOfferUfrag(offer []byte) string {
	var desc sdp.SessionDescription
	if err := desc.Unmarshal(offer); err != nil {
		return ""
	}

	// per-media credentials (priority matches sdpFragmentToCredentials)
	for _, media := range desc.MediaDescriptions {
		if ufrag, ok := media.Attribute("ice-ufrag"); ok && ufrag != "" {
			return ufrag
		}
	}

	// session-level credentials
	for _, attr := range desc.Attributes {
		if attr.Key == "ice-ufrag" && attr.Value != "" {
			return attr.Value
		}
	}
	return ""
}

func replaceICECredentials(offerSDP []byte, ufrag, pwd string) []byte {
	s := string(offerSDP)
	sep := "\r\n"
	if !strings.Contains(s, "\r\n") {
		sep = "\n"
	}
	lines := strings.Split(s, sep)
	for i, line := range lines {
		if strings.HasPrefix(line, "a=ice-ufrag:") {
			lines[i] = "a=ice-ufrag:" + ufrag
		} else if strings.HasPrefix(line, "a=ice-pwd:") {
			lines[i] = "a=ice-pwd:" + pwd
		}
	}
	return []byte(strings.Join(lines, sep))
}

func sdpFragmentToCredentials(frag *whip.SDPFragment) (string, string, error) {
	// media credentials
	for _, media := range frag.Medias {
		ufrag, _ := media.Attribute("ice-ufrag")
		pwd, _ := media.Attribute("ice-pwd")
		if ufrag != "" && pwd != "" {
			return ufrag, pwd, nil
		}
	}

	// session-wide credentials
	var ufrag, pwd string
	for _, attr := range frag.Attributes {
		switch attr.Key {
		case "ice-ufrag":
			ufrag = attr.Value
		case "ice-pwd":
			pwd = attr.Value
		}
	}
	if ufrag != "" && pwd != "" {
		return ufrag, pwd, nil
	}

	return "", "", fmt.Errorf("ICE credentials not found")
}

func sdpFragmentToCandidates(frag *whip.SDPFragment) ([]*pwebrtc.ICECandidateInit, error) {
	var candidates []*pwebrtc.ICECandidateInit

	for _, media := range frag.Medias {
		mid, ok := media.Attribute("mid")
		if !ok {
			return nil, fmt.Errorf("mid attribute is missing")
		}

		tmp, err := strconv.ParseUint(mid, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid mid attribute")
		}
		midNum := uint16(tmp)

		for _, attr := range media.Attributes {
			if attr.Key == "candidate" {
				candidates = append(candidates, &pwebrtc.ICECandidateInit{
					Candidate:     attr.Value,
					SDPMid:        &mid,
					SDPMLineIndex: &midNum,
				})
			}
		}
	}

	return candidates, nil
}

func mediaHasCredentialsOrCandidates(media *sdp.MediaDescription) bool {
	hasUfrag := false
	hasPwd := false

	for _, attr := range media.Attributes {
		if attr.Value != "" {
			switch attr.Key {
			case "ice-ufrag":
				hasUfrag = true

			case "ice-pwd":
				hasPwd = true

			case "candidate":
				return true
			}
		}
	}

	return (hasUfrag && hasPwd)
}

func fullAnswerToSDPFragment(answerSDP string) (*whip.SDPFragment, error) {
	var psdp sdp.SessionDescription
	err := psdp.Unmarshal([]byte(answerSDP))
	if err != nil {
		return nil, err
	}

	frag := &whip.SDPFragment{
		Attributes: []sdp.Attribute{
			{Key: "ice-options", Value: "trickle ice2"},
		},
	}

	filled := false

	for _, attr := range psdp.Attributes {
		switch attr.Key {
		case "ice-ufrag", "ice-pwd":
			frag.Attributes = append(frag.Attributes, sdp.Attribute{Key: attr.Key, Value: attr.Value})
			filled = true
		}
	}

	for _, media := range psdp.MediaDescriptions {
		if mediaHasCredentialsOrCandidates(media) {
			filled = true

			mid, ok := media.Attribute("mid")
			if !ok {
				return nil, fmt.Errorf("mid attribute is missing")
			}

			mediaFrag := &sdp.MediaDescription{
				MediaName: media.MediaName,
				Attributes: []sdp.Attribute{
					{Key: "mid", Value: mid},
				},
			}

			ufrag, _ := media.Attribute("ice-ufrag")
			pwd, _ := media.Attribute("ice-pwd")
			if ufrag != "" && pwd != "" {
				mediaFrag.Attributes = append(mediaFrag.Attributes, sdp.Attribute{Key: "ice-ufrag", Value: ufrag})
				mediaFrag.Attributes = append(mediaFrag.Attributes, sdp.Attribute{Key: "ice-pwd", Value: pwd})
			}

			for _, attr := range media.Attributes {
				if attr.Key == "candidate" {
					mediaFrag.Attributes = append(mediaFrag.Attributes, attr)
				}
			}
			mediaFrag.Attributes = append(mediaFrag.Attributes, sdp.Attribute{Key: "end-of-candidates"})

			frag.Medias = append(frag.Medias, mediaFrag)
		}
	}

	if !filled {
		return nil, fmt.Errorf("no credentials or candidates found in the answer")
	}

	return frag, nil
}

type sessionParent interface {
	closeSession(sx *session)
	generateICEServers(clientConfig bool) ([]pwebrtc.ICEServer, error)
	logger.Writer
}

type session struct {
	udpReadBufferSize     uint
	parentCtx             context.Context
	ipsFromInterfaces     bool
	ipsFromInterfacesList []string
	additionalHosts       []string
	iceUDPMux             ice.UDPMux
	iceTCPMux             *webrtc.TCPMuxWrapper
	stunGatherTimeout     conf.Duration
	handshakeTimeout      conf.Duration
	trackGatherTimeout    conf.Duration
	req                   webRTCNewSessionReq
	wg                    *sync.WaitGroup
	externalCmdPool       *externalcmd.Pool
	pathManager           serverPathManager
	parent                sessionParent

	ctx       context.Context
	ctxCancel func()
	created   time.Time
	uuid      uuid.UUID
	secret    uuid.UUID
	mutex     sync.RWMutex
	reader    *stream.Reader
	pc        *webrtc.PeerConnection
	user      string

	chNew           chan webRTCNewSessionReq
	chAddCandidates chan webRTCAddSessionCandidatesReq
}

func (s *session) initialize() {
	ctx, ctxCancel := context.WithCancel(s.parentCtx)

	s.ctx = ctx
	s.ctxCancel = ctxCancel
	s.created = time.Now()
	s.uuid = uuid.New()
	s.secret = uuid.New()
	s.chNew = make(chan webRTCNewSessionReq)
	s.chAddCandidates = make(chan webRTCAddSessionCandidatesReq)

	s.Log(logger.Info, "created by %s", s.req.remoteAddr)

	s.wg.Add(1)

	go s.run()
}

// Log implements logger.Writer.
func (s *session) Log(level logger.Level, format string, args ...any) {
	id := hex.EncodeToString(s.uuid[:4])
	s.parent.Log(level, "[session %v] "+format, append([]any{id}, args...)...)
}

func (s *session) Close() {
	s.ctxCancel()
}

func (s *session) run() {
	defer s.wg.Done()

	err := s.runInner()

	s.ctxCancel()

	s.parent.closeSession(s)

	s.Log(logger.Info, "closed: %v", err)
}

func (s *session) runInner() error {
	select {
	case <-s.chNew:
	case <-s.ctx.Done():
		return fmt.Errorf("terminated")
	}

	errStatusCode, err := s.runInner2()

	if errStatusCode != 0 {
		s.req.res <- webRTCNewSessionRes{
			errStatusCode: errStatusCode,
			err:           err,
		}
	}

	return err
}

func (s *session) runInner2() (int, error) {
	if s.req.publish {
		return s.runPublish()
	}
	return s.runRead()
}

func (s *session) runPublish() (int, error) {
	ip, _, _ := net.SplitHostPort(s.req.remoteAddr)

	res1, err := s.pathManager.FindPathConf(defs.PathFindPathConfReq{
		AccessRequest: defs.PathAccessRequest{
			Name:        s.req.pathName,
			Query:       s.req.httpRequest.URL.RawQuery,
			Publish:     true,
			Proto:       auth.ProtocolWebRTC,
			ID:          &s.uuid,
			Credentials: httpp.Credentials(s.req.httpRequest),
			IP:          net.ParseIP(ip),
		},
	})
	if err != nil {
		return http.StatusBadRequest, err
	}

	s.mutex.Lock()
	s.user = res1.User
	s.mutex.Unlock()

	iceServers, err := s.parent.generateICEServers(false)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	pc := &webrtc.PeerConnection{
		UDPReadBufferSize:     s.udpReadBufferSize,
		ICEUDPMux:             s.iceUDPMux,
		ICETCPMux:             s.iceTCPMux,
		ICEServers:            iceServers,
		IPsFromInterfaces:     s.ipsFromInterfaces,
		IPsFromInterfacesList: s.ipsFromInterfacesList,
		AdditionalHosts:       s.additionalHosts,
		STUNGatherTimeout:     time.Duration(s.stunGatherTimeout),
		Publish:               false,
		Log:                   s,
	}
	err = pc.Start()
	if err != nil {
		return http.StatusBadRequest, err
	}

	terminatorDone := make(chan struct{})
	defer func() { <-terminatorDone }()

	terminatorRun := make(chan struct{})
	defer close(terminatorRun)

	go func() {
		defer close(terminatorDone)
		select {
		case <-s.ctx.Done():
		case <-terminatorRun:
		}
		pc.Close()
	}()

	offer := whipOffer(s.req.offer)

	var sdp sdp.SessionDescription
	err = sdp.Unmarshal([]byte(offer.SDP))
	if err != nil {
		return http.StatusBadRequest, err
	}

	err = webrtc.TracksAreValid(sdp.MediaDescriptions)
	if err != nil {
		// RFC draft-ietf-wish-whip
		// if the number of audio and or video
		// tracks or number streams is not supported by the WHIP Endpoint, it
		// MUST reject the HTTP POST request with a "406 Not Acceptable" error
		// response.
		return http.StatusNotAcceptable, err
	}

	answer, err := pc.CreateFullAnswer(offer, false)
	if err != nil {
		return http.StatusBadRequest, err
	}

	s.writeAnswer(answer)

	go s.readRemoteCandidates(pc)

	err = pc.WaitUntilConnected(time.Duration(s.handshakeTimeout))
	if err != nil {
		return 0, err
	}

	s.mutex.Lock()
	s.pc = pc
	s.mutex.Unlock()

	err = pc.GatherIncomingTracks(time.Duration(s.trackGatherTimeout))
	if err != nil {
		return 0, err
	}

	var subStream *stream.SubStream

	medias, err := webrtc.ToStream(pc, res1.Conf, &subStream, s)
	if err != nil {
		return 0, err
	}

	res2, err := s.pathManager.AddPublisher(defs.PathAddPublisherReq{
		Author:        s,
		Desc:          &description.Session{Medias: medias},
		UseRTPPackets: true,
		ReplaceNTP:    !res1.Conf.UseAbsoluteTimestamp,
		ConfToCompare: res1.Conf,
		AccessRequest: defs.PathAccessRequest{
			Name:     s.req.pathName,
			Query:    s.req.httpRequest.URL.RawQuery,
			Publish:  true,
			SkipAuth: true,
		},
	})
	if err != nil {
		return 0, err
	}

	defer res2.Path.RemovePublisher(defs.PathRemovePublisherReq{Author: s})

	subStream = res2.SubStream

	pc.StartReading()

	select {
	case <-pc.Failed():
		return 0, fmt.Errorf("peer connection closed")

	case <-s.ctx.Done():
		return 0, fmt.Errorf("terminated")
	}
}

func (s *session) runRead() (int, error) {
	ip, _, _ := net.SplitHostPort(s.req.remoteAddr)

	res, err := s.pathManager.AddReader(defs.PathAddReaderReq{
		Author: s,
		AccessRequest: defs.PathAccessRequest{
			Name:        s.req.pathName,
			Query:       s.req.httpRequest.URL.RawQuery,
			Proto:       auth.ProtocolWebRTC,
			ID:          &s.uuid,
			Credentials: httpp.Credentials(s.req.httpRequest),
			IP:          net.ParseIP(ip),
		},
	})
	if err != nil {
		var terr2 *defs.PathNoStreamAvailableError
		if errors.As(err, &terr2) {
			return http.StatusNotFound, err
		}

		return http.StatusBadRequest, err
	}

	defer res.Path.RemoveReader(defs.PathRemoveReaderReq{Author: s})

	s.mutex.Lock()
	s.user = res.User
	s.mutex.Unlock()

	iceServers, err := s.parent.generateICEServers(false)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	pc := &webrtc.PeerConnection{
		UDPReadBufferSize:     s.udpReadBufferSize,
		ICEUDPMux:             s.iceUDPMux,
		ICETCPMux:             s.iceTCPMux,
		ICEServers:            iceServers,
		IPsFromInterfaces:     s.ipsFromInterfaces,
		IPsFromInterfacesList: s.ipsFromInterfacesList,
		AdditionalHosts:       s.additionalHosts,
		STUNGatherTimeout:     time.Duration(s.stunGatherTimeout),
		Publish:               true,
		Log:                   s,
	}

	r := &stream.Reader{Parent: s}

	err = webrtc.FromStream(res.Stream.Desc, r, pc)
	if err != nil {
		return http.StatusBadRequest, err
	}

	err = pc.Start()
	if err != nil {
		return http.StatusBadRequest, err
	}

	terminatorDone := make(chan struct{})
	defer func() { <-terminatorDone }()

	terminatorRun := make(chan struct{})
	defer close(terminatorRun)

	go func() {
		defer close(terminatorDone)
		select {
		case <-s.ctx.Done():
		case <-terminatorRun:
		}
		pc.Close()
	}()

	offer := whipOffer(s.req.offer)

	answer, err := pc.CreateFullAnswer(offer, false)
	if err != nil {
		return http.StatusBadRequest, err
	}

	s.writeAnswer(answer)

	go s.readRemoteCandidates(pc)

	err = pc.WaitUntilConnected(time.Duration(s.handshakeTimeout))
	if err != nil {
		return 0, err
	}

	s.mutex.Lock()
	s.pc = pc
	s.mutex.Unlock()

	s.Log(logger.Info, "is reading from path '%s', %s",
		res.Path.Name(), defs.FormatsInfo(r.Formats()))

	onUnreadHook := hooks.OnRead(hooks.OnReadParams{
		Logger:          s,
		ExternalCmdPool: s.externalCmdPool,
		Conf:            res.Path.SafeConf(),
		ExternalCmdEnv:  res.Path.ExternalCmdEnv(),
		Reader:          *s.APIReaderDescribe(),
		Query:           s.req.httpRequest.URL.RawQuery,
	})
	defer onUnreadHook()

	res.Stream.AddReader(r)
	defer res.Stream.RemoveReader(r)

	s.mutex.Lock()
	s.reader = r
	s.mutex.Unlock()

	select {
	case <-pc.Failed():
		return 0, fmt.Errorf("peer connection closed")

	case err = <-r.Error():
		return 0, err

	case <-s.ctx.Done():
		return 0, fmt.Errorf("terminated")
	}
}

func (s *session) writeAnswer(answer *pwebrtc.SessionDescription) {
	s.req.res <- webRTCNewSessionRes{
		sx:     s,
		answer: []byte(answer.SDP),
	}
}

func (s *session) readRemoteCandidates(pc *webrtc.PeerConnection) {
	remoteUfrag := parseOfferUfrag(s.req.offer)

	for {
		select {
		case req := <-s.chAddCandidates:
			// do not check for errors since credentials are optional
			ufrag, pwd, _ := sdpFragmentToCredentials(req.fragment)

			candidates, err := sdpFragmentToCandidates(req.fragment)
			if err != nil {
				req.res <- webRTCAddSessionCandidatesRes{err: err}
				continue
			}

			// ICE restart: client sent new credentials
			var answer *pwebrtc.SessionDescription
			if ufrag != "" && ufrag != remoteUfrag {
				sdp := replaceICECredentials(s.req.offer, ufrag, pwd)

				answer, err = pc.CreateFullAnswer(whipOffer(sdp), true)
				if err != nil {
					req.res <- webRTCAddSessionCandidatesRes{err: err}
					continue
				}
			}

			var addErr error
			for _, candidate := range candidates {
				addErr = pc.AddRemoteCandidate(candidate)
				if addErr != nil {
					break
				}
			}
			if addErr != nil {
				req.res <- webRTCAddSessionCandidatesRes{err: addErr}
				continue
			}

			if ufrag != "" && ufrag != remoteUfrag {
				var frag *whip.SDPFragment
				frag, err = fullAnswerToSDPFragment(answer.SDP)
				if err != nil {
					req.res <- webRTCAddSessionCandidatesRes{err: err}
					continue
				}

				remoteUfrag = ufrag
				req.res <- webRTCAddSessionCandidatesRes{answer: frag}
			} else {
				req.res <- webRTCAddSessionCandidatesRes{}
			}

		case <-s.ctx.Done():
			return
		}
	}
}

// new is called by webRTCHTTPServer through Server.
func (s *session) new(req webRTCNewSessionReq) webRTCNewSessionRes {
	select {
	case s.chNew <- req:
		return <-req.res

	case <-s.ctx.Done():
		return webRTCNewSessionRes{err: fmt.Errorf("terminated"), errStatusCode: http.StatusInternalServerError}
	}
}

// addCandidates is called by webRTCHTTPServer through Server.
func (s *session) addCandidates(
	req webRTCAddSessionCandidatesReq,
) webRTCAddSessionCandidatesRes {
	select {
	case s.chAddCandidates <- req:
		return <-req.res

	case <-s.ctx.Done():
		return webRTCAddSessionCandidatesRes{err: fmt.Errorf("terminated")}
	}
}

// APIReaderDescribe implements reader.
func (s *session) APIReaderDescribe() *defs.APIPathReader {
	return &defs.APIPathReader{
		Type: defs.APIPathReaderTypeWebRTCSession,
		ID:   s.uuid.String(),
	}
}

// APISourceDescribe implements source.
func (s *session) APISourceDescribe() *defs.APIPathSource {
	return &defs.APIPathSource{
		Type: defs.APIPathSourceTypeWebRTCSession,
		ID:   s.uuid.String(),
	}
}

func (s *session) apiItem() *defs.APIWebRTCSession {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	peerConnectionEstablished := false
	localCandidate := ""
	remoteCandidate := ""
	bytesReceived := uint64(0)
	bytesSent := uint64(0)
	rtpPacketsReceived := uint64(0)
	rtpPacketsSent := uint64(0)
	rtpPacketsLost := uint64(0)
	rtpPacketsJitter := float64(0)
	rtcpPacketsReceived := uint64(0)
	rtcpPacketsSent := uint64(0)
	outboundFramesDiscarded := uint64(0)

	if s.pc != nil {
		peerConnectionEstablished = true
		localCandidate = s.pc.LocalCandidate()
		remoteCandidate = s.pc.RemoteCandidate()
		stats := s.pc.Stats()
		bytesReceived = stats.BytesReceived
		bytesSent = stats.BytesSent
		rtpPacketsReceived = stats.RTPPacketsReceived
		rtpPacketsSent = stats.RTPPacketsSent
		rtpPacketsLost = stats.RTPPacketsLost
		rtpPacketsJitter = stats.RTPPacketsJitter
		rtcpPacketsReceived = stats.RTCPPacketsReceived
		rtcpPacketsSent = stats.RTCPPacketsSent
	}

	if s.reader != nil {
		outboundFramesDiscarded = s.reader.OutboundFramesDiscarded()
	}

	return &defs.APIWebRTCSession{
		ID:                        s.uuid,
		Created:                   s.created,
		RemoteAddr:                s.req.remoteAddr,
		PeerConnectionEstablished: peerConnectionEstablished,
		LocalCandidate:            localCandidate,
		RemoteCandidate:           remoteCandidate,
		State: func() defs.APIWebRTCSessionState {
			if s.req.publish {
				return defs.APIWebRTCSessionStatePublish
			}
			return defs.APIWebRTCSessionStateRead
		}(),
		Path:                    s.req.pathName,
		Query:                   s.req.httpRequest.URL.RawQuery,
		User:                    s.user,
		InboundBytes:            bytesReceived,
		InboundRTPPackets:       rtpPacketsReceived,
		InboundRTPPacketsLost:   rtpPacketsLost,
		InboundRTPPacketsJitter: rtpPacketsJitter,
		InboundRTCPPackets:      rtcpPacketsReceived,
		OutboundBytes:           bytesSent,
		OutboundRTPPackets:      rtpPacketsSent,
		OutboundRTCPPackets:     rtcpPacketsSent,
		OutboundFramesDiscarded: outboundFramesDiscarded,
		BytesReceived:           bytesReceived,
		BytesSent:               bytesSent,
		RTPPacketsReceived:      rtpPacketsReceived,
		RTPPacketsSent:          rtpPacketsSent,
		RTPPacketsLost:          rtpPacketsLost,
		RTPPacketsJitter:        rtpPacketsJitter,
		RTCPPacketsReceived:     rtcpPacketsReceived,
		RTCPPacketsSent:         rtcpPacketsSent,
	}
}
