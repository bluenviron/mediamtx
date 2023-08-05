package core

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/bluenviron/gortsplib/v3/pkg/ringbuffer"
	"github.com/google/uuid"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/webrtcpc"
)

type trackRecvPair struct {
	track    *webrtc.TrackRemote
	receiver *webrtc.RTPReceiver
}

func webrtcMediasOfOutgoingTracks(tracks []*webRTCOutgoingTrack) media.Medias {
	ret := make(media.Medias, len(tracks))
	for i, track := range tracks {
		ret[i] = track.media
	}
	return ret
}

func webrtcMediasOfIncomingTracks(tracks []*webRTCIncomingTrack) media.Medias {
	ret := make(media.Medias, len(tracks))
	for i, track := range tracks {
		ret[i] = track.media
	}
	return ret
}

func whipOffer(body []byte) *webrtc.SessionDescription {
	return &webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  string(body),
	}
}

func webrtcWaitUntilConnected(
	ctx context.Context,
	pc *webrtcpc.PeerConnection,
) error {
	t := time.NewTimer(webrtcHandshakeTimeout)
	defer t.Stop()

outer:
	for {
		select {
		case <-t.C:
			return fmt.Errorf("deadline exceeded while waiting connection")

		case <-pc.Connected():
			break outer

		case <-ctx.Done():
			return fmt.Errorf("terminated")
		}
	}

	return nil
}

func webrtcGatherOutgoingTracks(medias media.Medias) ([]*webRTCOutgoingTrack, error) {
	var tracks []*webRTCOutgoingTrack

	videoTrack, err := newWebRTCOutgoingTrackVideo(medias)
	if err != nil {
		return nil, err
	}

	if videoTrack != nil {
		tracks = append(tracks, videoTrack)
	}

	audioTrack, err := newWebRTCOutgoingTrackAudio(medias)
	if err != nil {
		return nil, err
	}

	if audioTrack != nil {
		tracks = append(tracks, audioTrack)
	}

	if tracks == nil {
		return nil, fmt.Errorf(
			"the stream doesn't contain any supported codec, which are currently AV1, VP9, VP8, H264, Opus, G722, G711")
	}

	return tracks, nil
}

func webrtcTrackCount(medias []*sdp.MediaDescription) (int, error) {
	videoTrack := false
	audioTrack := false
	trackCount := 0

	for _, media := range medias {
		switch media.MediaName.Media {
		case "video":
			if videoTrack {
				return 0, fmt.Errorf("only a single video and a single audio track are supported")
			}
			videoTrack = true

		case "audio":
			if audioTrack {
				return 0, fmt.Errorf("only a single video and a single audio track are supported")
			}
			audioTrack = true

		default:
			return 0, fmt.Errorf("unsupported media '%s'", media.MediaName.Media)
		}

		trackCount++
	}

	return trackCount, nil
}

func webrtcGatherIncomingTracks(
	ctx context.Context,
	pc *webrtcpc.PeerConnection,
	trackRecv chan trackRecvPair,
	trackCount int,
) ([]*webRTCIncomingTrack, error) {
	var tracks []*webRTCIncomingTrack

	t := time.NewTimer(webrtcTrackGatherTimeout)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			if trackCount == 0 {
				return tracks, nil
			}
			return nil, fmt.Errorf("deadline exceeded while waiting tracks")

		case pair := <-trackRecv:
			track, err := newWebRTCIncomingTrack(pair.track, pair.receiver, pc.WriteRTCP)
			if err != nil {
				return nil, err
			}
			tracks = append(tracks, track)

			if len(tracks) == trackCount {
				return tracks, nil
			}

		case <-pc.Disconnected():
			return nil, fmt.Errorf("peer connection closed")

		case <-ctx.Done():
			return nil, fmt.Errorf("terminated")
		}
	}
}

type webRTCSessionPathManager interface {
	addPublisher(req pathAddPublisherReq) pathAddPublisherRes
	addReader(req pathAddReaderReq) pathAddReaderRes
}

type webRTCSession struct {
	readBufferCount int
	api             *webrtc.API
	req             webRTCNewSessionReq
	wg              *sync.WaitGroup
	pathManager     webRTCSessionPathManager
	parent          *webRTCManager

	ctx       context.Context
	ctxCancel func()
	created   time.Time
	uuid      uuid.UUID
	secret    uuid.UUID
	mutex     sync.RWMutex
	pc        *webrtcpc.PeerConnection

	chNew           chan webRTCNewSessionReq
	chAddCandidates chan webRTCAddSessionCandidatesReq
}

func newWebRTCSession(
	parentCtx context.Context,
	readBufferCount int,
	api *webrtc.API,
	req webRTCNewSessionReq,
	wg *sync.WaitGroup,
	pathManager webRTCSessionPathManager,
	parent *webRTCManager,
) *webRTCSession {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	s := &webRTCSession{
		readBufferCount: readBufferCount,
		api:             api,
		req:             req,
		wg:              wg,
		parent:          parent,
		pathManager:     pathManager,
		ctx:             ctx,
		ctxCancel:       ctxCancel,
		created:         time.Now(),
		uuid:            uuid.New(),
		secret:          uuid.New(),
		chNew:           make(chan webRTCNewSessionReq),
		chAddCandidates: make(chan webRTCAddSessionCandidatesReq),
	}

	s.Log(logger.Info, "created by %s", req.remoteAddr)

	wg.Add(1)
	go s.run()

	return s
}

func (s *webRTCSession) Log(level logger.Level, format string, args ...interface{}) {
	id := hex.EncodeToString(s.uuid[:4])
	s.parent.Log(level, "[session %v] "+format, append([]interface{}{id}, args...)...)
}

func (s *webRTCSession) close() {
	s.ctxCancel()
}

func (s *webRTCSession) run() {
	defer s.wg.Done()

	err := s.runInner()

	s.ctxCancel()

	s.parent.closeSession(s)

	s.Log(logger.Info, "closed (%v)", err)
}

func (s *webRTCSession) runInner() error {
	select {
	case <-s.chNew:
	case <-s.ctx.Done():
		return fmt.Errorf("terminated")
	}

	errStatusCode, err := s.runInner2()

	if errStatusCode != 0 {
		s.req.res <- webRTCNewSessionRes{
			err:           err,
			errStatusCode: errStatusCode,
		}
	}

	return err
}

func (s *webRTCSession) runInner2() (int, error) {
	if s.req.publish {
		return s.runPublish()
	}
	return s.runRead()
}

func (s *webRTCSession) runPublish() (int, error) {
	ip, _, _ := net.SplitHostPort(s.req.remoteAddr)

	res := s.pathManager.addPublisher(pathAddPublisherReq{
		author:   s,
		pathName: s.req.pathName,
		credentials: authCredentials{
			query: s.req.query,
			ip:    net.ParseIP(ip),
			user:  s.req.user,
			pass:  s.req.pass,
			proto: authProtocolWebRTC,
			id:    &s.uuid,
		},
	})
	if res.err != nil {
		if _, ok := res.err.(*errAuthentication); ok {
			// wait some seconds to stop brute force attacks
			<-time.After(webrtcPauseAfterAuthError)

			return http.StatusUnauthorized, res.err
		}

		return http.StatusBadRequest, res.err
	}

	defer res.path.removePublisher(pathRemovePublisherReq{author: s})

	servers, err := s.parent.generateICEServers()
	if err != nil {
		return http.StatusInternalServerError, err
	}

	pc, err := webrtcpc.New(
		servers,
		s.api,
		s)
	if err != nil {
		return http.StatusBadRequest, err
	}
	defer pc.Close()

	offer := whipOffer(s.req.offer)

	var sdp sdp.SessionDescription
	err = sdp.Unmarshal([]byte(offer.SDP))
	if err != nil {
		return http.StatusBadRequest, err
	}

	trackCount, err := webrtcTrackCount(sdp.MediaDescriptions)
	if err != nil {
		return http.StatusBadRequest, err
	}

	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RtpTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		return http.StatusBadRequest, err
	}

	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RtpTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		return http.StatusBadRequest, err
	}

	trackRecv := make(chan trackRecvPair)

	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		select {
		case trackRecv <- trackRecvPair{track, receiver}:
		case <-s.ctx.Done():
		}
	})

	err = pc.SetRemoteDescription(*offer)
	if err != nil {
		return http.StatusBadRequest, err
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return http.StatusBadRequest, err
	}

	err = pc.SetLocalDescription(answer)
	if err != nil {
		return http.StatusBadRequest, err
	}

	err = pc.WaitGatheringDone(s.ctx)
	if err != nil {
		return http.StatusBadRequest, err
	}

	s.writeAnswer(pc.LocalDescription())

	go s.readRemoteCandidates(pc)

	err = webrtcWaitUntilConnected(s.ctx, pc)
	if err != nil {
		return 0, err
	}

	s.mutex.Lock()
	s.pc = pc
	s.mutex.Unlock()

	tracks, err := webrtcGatherIncomingTracks(s.ctx, pc, trackRecv, trackCount)
	if err != nil {
		return 0, err
	}
	medias := webrtcMediasOfIncomingTracks(tracks)

	rres := res.path.startPublisher(pathStartPublisherReq{
		author:             s,
		medias:             medias,
		generateRTPPackets: false,
	})
	if rres.err != nil {
		return 0, rres.err
	}

	for _, track := range tracks {
		track.start(rres.stream)
	}

	select {
	case <-pc.Disconnected():
		return 0, fmt.Errorf("peer connection closed")

	case <-s.ctx.Done():
		return 0, fmt.Errorf("terminated")
	}
}

func (s *webRTCSession) runRead() (int, error) {
	ip, _, _ := net.SplitHostPort(s.req.remoteAddr)

	res := s.pathManager.addReader(pathAddReaderReq{
		author:   s,
		pathName: s.req.pathName,
		credentials: authCredentials{
			query: s.req.query,
			ip:    net.ParseIP(ip),
			user:  s.req.user,
			pass:  s.req.pass,
			proto: authProtocolWebRTC,
			id:    &s.uuid,
		},
	})
	if res.err != nil {
		if _, ok := res.err.(*errAuthentication); ok {
			// wait some seconds to stop brute force attacks
			<-time.After(webrtcPauseAfterAuthError)

			return http.StatusUnauthorized, res.err
		}

		if strings.HasPrefix(res.err.Error(), "no one is publishing") {
			return http.StatusNotFound, res.err
		}

		return http.StatusBadRequest, res.err
	}

	defer res.path.removeReader(pathRemoveReaderReq{author: s})

	tracks, err := webrtcGatherOutgoingTracks(res.stream.Medias())
	if err != nil {
		return http.StatusBadRequest, err
	}

	servers, err := s.parent.generateICEServers()
	if err != nil {
		return http.StatusInternalServerError, err
	}

	pc, err := webrtcpc.New(
		servers,
		s.api,
		s)
	if err != nil {
		return http.StatusBadRequest, err
	}
	defer pc.Close()

	for _, track := range tracks {
		var err error
		track.sender, err = pc.AddTrack(track.track)
		if err != nil {
			return http.StatusBadRequest, err
		}
	}

	offer := whipOffer(s.req.offer)

	err = pc.SetRemoteDescription(*offer)
	if err != nil {
		return http.StatusBadRequest, err
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return http.StatusBadRequest, err
	}

	err = pc.SetLocalDescription(answer)
	if err != nil {
		return http.StatusBadRequest, err
	}

	err = pc.WaitGatheringDone(s.ctx)
	if err != nil {
		return http.StatusBadRequest, err
	}

	s.writeAnswer(pc.LocalDescription())

	go s.readRemoteCandidates(pc)

	err = webrtcWaitUntilConnected(s.ctx, pc)
	if err != nil {
		return 0, err
	}

	s.mutex.Lock()
	s.pc = pc
	s.mutex.Unlock()

	ringBuffer, _ := ringbuffer.New(uint64(s.readBufferCount))
	defer ringBuffer.Close()

	writeError := make(chan error)

	for _, track := range tracks {
		track.start(s.ctx, s, res.stream, ringBuffer, writeError)
	}

	defer res.stream.RemoveReader(s)

	s.Log(logger.Info, "is reading from path '%s', %s",
		res.path.name, sourceMediaInfo(webrtcMediasOfOutgoingTracks(tracks)))

	go func() {
		for {
			item, ok := ringBuffer.Pull()
			if !ok {
				return
			}
			item.(func())()
		}
	}()

	select {
	case <-pc.Disconnected():
		return 0, fmt.Errorf("peer connection closed")

	case err := <-writeError:
		return 0, err

	case <-s.ctx.Done():
		return 0, fmt.Errorf("terminated")
	}
}

func (s *webRTCSession) writeAnswer(answer *webrtc.SessionDescription) {
	s.req.res <- webRTCNewSessionRes{
		sx:     s,
		answer: []byte(answer.SDP),
	}
}

func (s *webRTCSession) readRemoteCandidates(pc *webrtcpc.PeerConnection) {
	for {
		select {
		case req := <-s.chAddCandidates:
			for _, candidate := range req.candidates {
				err := pc.AddICECandidate(*candidate)
				if err != nil {
					req.res <- webRTCAddSessionCandidatesRes{err: err}
				}
			}
			req.res <- webRTCAddSessionCandidatesRes{}

		case <-s.ctx.Done():
			return
		}
	}
}

// new is called by webRTCHTTPServer through webRTCManager.
func (s *webRTCSession) new(req webRTCNewSessionReq) webRTCNewSessionRes {
	select {
	case s.chNew <- req:
		return <-req.res

	case <-s.ctx.Done():
		return webRTCNewSessionRes{err: fmt.Errorf("terminated"), errStatusCode: http.StatusInternalServerError}
	}
}

// addCandidates is called by webRTCHTTPServer through webRTCManager.
func (s *webRTCSession) addCandidates(
	req webRTCAddSessionCandidatesReq,
) webRTCAddSessionCandidatesRes {
	select {
	case s.chAddCandidates <- req:
		return <-req.res

	case <-s.ctx.Done():
		return webRTCAddSessionCandidatesRes{err: fmt.Errorf("terminated")}
	}
}

// apiSourceDescribe implements sourceStaticImpl.
func (s *webRTCSession) apiSourceDescribe() pathAPISourceOrReader {
	return pathAPISourceOrReader{
		Type: "webRTCSession",
		ID:   s.uuid.String(),
	}
}

// apiReaderDescribe implements reader.
func (s *webRTCSession) apiReaderDescribe() pathAPISourceOrReader {
	return s.apiSourceDescribe()
}

func (s *webRTCSession) apiItem() *apiWebRTCSession {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	peerConnectionEstablished := false
	localCandidate := ""
	remoteCandidate := ""
	bytesReceived := uint64(0)
	bytesSent := uint64(0)

	if s.pc != nil {
		peerConnectionEstablished = true
		localCandidate = s.pc.LocalCandidate()
		remoteCandidate = s.pc.RemoteCandidate()
		bytesReceived = s.pc.BytesReceived()
		bytesSent = s.pc.BytesSent()
	}

	return &apiWebRTCSession{
		ID:                        s.uuid,
		Created:                   s.created,
		RemoteAddr:                s.req.remoteAddr,
		PeerConnectionEstablished: peerConnectionEstablished,
		LocalCandidate:            localCandidate,
		RemoteCandidate:           remoteCandidate,
		State: func() apiWebRTCSessionState {
			if s.req.publish {
				return apiWebRTCSessionStatePublish
			}
			return apiWebRTCSessionStateRead
		}(),
		Path:          s.req.pathName,
		BytesReceived: bytesReceived,
		BytesSent:     bytesSent,
	}
}
