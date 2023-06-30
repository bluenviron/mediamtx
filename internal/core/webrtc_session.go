package core

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/bluenviron/gortsplib/v3/pkg/ringbuffer"
	"github.com/google/uuid"
	"github.com/pion/ice/v2"
	"github.com/pion/webrtc/v3"

	"github.com/bluenviron/mediamtx/internal/logger"
)

type trackRecvPair struct {
	track    *webrtc.TrackRemote
	receiver *webrtc.RTPReceiver
}

func mediasOfOutgoingTracks(tracks []*webRTCOutgoingTrack) media.Medias {
	ret := make(media.Medias, len(tracks))
	for i, track := range tracks {
		ret[i] = track.media
	}
	return ret
}

func mediasOfIncomingTracks(tracks []*webRTCIncomingTrack) media.Medias {
	ret := make(media.Medias, len(tracks))
	for i, track := range tracks {
		ret[i] = track.media
	}
	return ret
}

func gatherOutgoingTracks(medias media.Medias) ([]*webRTCOutgoingTrack, error) {
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
			"the stream doesn't contain any supported codec, which are currently H264, VP8, VP9, G711, G722, Opus")
	}

	return tracks, nil
}

func gatherIncomingTracks(
	ctx context.Context,
	pc *peerConnection,
	trackRecv chan trackRecvPair,
) ([]*webRTCIncomingTrack, error) {
	var tracks []*webRTCIncomingTrack

	t := time.NewTimer(webrtcTrackGatherTimeout)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			if len(tracks) == 0 {
				return nil, fmt.Errorf("no tracks found")
			}
			return tracks, nil

		case pair := <-trackRecv:
			track, err := newWebRTCIncomingTrack(pair.track, pair.receiver, pc.WriteRTCP)
			if err != nil {
				return nil, err
			}
			tracks = append(tracks, track)

			if len(tracks) == 2 {
				return tracks, nil
			}

		case <-pc.disconnected:
			return nil, fmt.Errorf("peer connection closed")

		case <-ctx.Done():
			return nil, fmt.Errorf("terminated")
		}
	}
}

type webRTCSessionPathManager interface {
	publisherAdd(req pathPublisherAddReq) pathPublisherAnnounceRes
	readerAdd(req pathReaderAddReq) pathReaderSetupPlayRes
}

type webRTCSession struct {
	readBufferCount   int
	req               webRTCSessionNewReq
	wg                *sync.WaitGroup
	iceHostNAT1To1IPs []string
	iceUDPMux         ice.UDPMux
	iceTCPMux         ice.TCPMux
	pathManager       webRTCSessionPathManager
	parent            *webRTCManager

	ctx        context.Context
	ctxCancel  func()
	created    time.Time
	uuid       uuid.UUID
	secret     uuid.UUID
	pcMutex    sync.RWMutex
	pc         *peerConnection
	answerSent bool

	chAddRemoteCandidates chan webRTCSessionAddCandidatesReq
}

func newWebRTCSession(
	parentCtx context.Context,
	readBufferCount int,
	req webRTCSessionNewReq,
	wg *sync.WaitGroup,
	iceHostNAT1To1IPs []string,
	iceUDPMux ice.UDPMux,
	iceTCPMux ice.TCPMux,
	pathManager webRTCSessionPathManager,
	parent *webRTCManager,
) *webRTCSession {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	s := &webRTCSession{
		readBufferCount:       readBufferCount,
		req:                   req,
		wg:                    wg,
		iceHostNAT1To1IPs:     iceHostNAT1To1IPs,
		iceUDPMux:             iceUDPMux,
		iceTCPMux:             iceTCPMux,
		parent:                parent,
		pathManager:           pathManager,
		ctx:                   ctx,
		ctxCancel:             ctxCancel,
		created:               time.Now(),
		uuid:                  uuid.New(),
		secret:                uuid.New(),
		chAddRemoteCandidates: make(chan webRTCSessionAddCandidatesReq),
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

func (s *webRTCSession) safePC() *peerConnection {
	s.pcMutex.RLock()
	defer s.pcMutex.RUnlock()
	return s.pc
}

func (s *webRTCSession) run() {
	defer s.wg.Done()

	errStatusCode, err := s.runInner()

	if !s.answerSent {
		select {
		case s.req.res <- webRTCSessionNewRes{
			err:           err,
			errStatusCode: errStatusCode,
		}:
		case <-s.ctx.Done():
		}
	}

	s.ctxCancel()

	s.parent.sessionClose(s)

	s.Log(logger.Info, "closed (%v)", err)
}

func (s *webRTCSession) runInner() (int, error) {
	if s.req.publish {
		return s.runPublish()
	}
	return s.runRead()
}

func (s *webRTCSession) runPublish() (int, error) {
	res := s.pathManager.publisherAdd(pathPublisherAddReq{
		author:   s,
		pathName: s.req.pathName,
		skipAuth: true,
	})
	if res.err != nil {
		return http.StatusInternalServerError, res.err
	}

	defer res.path.publisherRemove(pathPublisherRemoveReq{author: s})

	pc, err := newPeerConnection(
		s.parent.generateICEServers(),
		s.iceHostNAT1To1IPs,
		s.iceUDPMux,
		s.iceTCPMux,
		s)
	if err != nil {
		return http.StatusBadRequest, err
	}
	defer pc.close()

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
		case <-pc.closed:
		}
	})

	offer := s.buildOffer()
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

	err = s.waitGatheringDone(pc)
	if err != nil {
		return http.StatusBadRequest, err
	}

	tmp := pc.LocalDescription()
	answer = *tmp

	err = s.writeAnswer(&answer)
	if err != nil {
		return http.StatusBadRequest, err
	}

	go s.readRemoteCandidates(pc)

	err = s.waitUntilConnected(pc)
	if err != nil {
		return 0, err
	}

	tracks, err := gatherIncomingTracks(s.ctx, pc, trackRecv)
	if err != nil {
		return 0, err
	}
	medias := mediasOfIncomingTracks(tracks)

	rres := res.path.publisherStart(pathPublisherStartReq{
		author:             s,
		medias:             medias,
		generateRTPPackets: false,
	})
	if rres.err != nil {
		return 0, rres.err
	}

	s.Log(logger.Info, "is publishing to path '%s', %s",
		res.path.name,
		sourceMediaInfo(medias))

	for _, track := range tracks {
		track.start(rres.stream)
	}

	select {
	case <-pc.disconnected:
		return 0, fmt.Errorf("peer connection closed")

	case <-s.ctx.Done():
		return 0, fmt.Errorf("terminated")
	}
}

func (s *webRTCSession) runRead() (int, error) {
	res := s.pathManager.readerAdd(pathReaderAddReq{
		author:   s,
		pathName: s.req.pathName,
		skipAuth: true,
	})
	if res.err != nil {
		if strings.HasPrefix(res.err.Error(), "no one is publishing") {
			return http.StatusNotFound, res.err
		}
		return http.StatusInternalServerError, res.err
	}

	defer res.path.readerRemove(pathReaderRemoveReq{author: s})

	tracks, err := gatherOutgoingTracks(res.stream.medias())
	if err != nil {
		return http.StatusBadRequest, err
	}

	pc, err := newPeerConnection(
		s.parent.generateICEServers(),
		s.iceHostNAT1To1IPs,
		s.iceUDPMux,
		s.iceTCPMux,
		s)
	if err != nil {
		return http.StatusBadRequest, err
	}
	defer pc.close()

	for _, track := range tracks {
		var err error
		track.sender, err = pc.AddTrack(track.track)
		if err != nil {
			return http.StatusBadRequest, err
		}
	}

	offer := s.buildOffer()
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

	err = s.waitGatheringDone(pc)
	if err != nil {
		return http.StatusBadRequest, err
	}

	tmp := pc.LocalDescription()
	answer = *tmp

	err = s.writeAnswer(&answer)
	if err != nil {
		return http.StatusBadRequest, err
	}

	go s.readRemoteCandidates(pc)

	err = s.waitUntilConnected(pc)
	if err != nil {
		return 0, err
	}

	ringBuffer, _ := ringbuffer.New(uint64(s.readBufferCount))
	defer ringBuffer.Close()

	writeError := make(chan error)

	for _, track := range tracks {
		track.start(s.ctx, s, res.stream, ringBuffer, writeError)
	}

	defer res.stream.readerRemove(s)

	s.Log(logger.Info, "is reading from path '%s', %s",
		res.path.name, sourceMediaInfo(mediasOfOutgoingTracks(tracks)))

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
	case <-pc.disconnected:
		return 0, fmt.Errorf("peer connection closed")

	case err := <-writeError:
		return 0, err

	case <-s.ctx.Done():
		return 0, fmt.Errorf("terminated")
	}
}

func (s *webRTCSession) buildOffer() *webrtc.SessionDescription {
	return &webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  string(s.req.offer),
	}
}

func (s *webRTCSession) waitGatheringDone(pc *peerConnection) error {
	for {
		select {
		case <-pc.localCandidateRecv:
		case <-pc.gatheringDone:
			return nil
		case <-s.ctx.Done():
			return fmt.Errorf("terminated")
		}
	}
}

func (s *webRTCSession) writeAnswer(answer *webrtc.SessionDescription) error {
	select {
	case s.req.res <- webRTCSessionNewRes{
		sx:     s,
		answer: []byte(answer.SDP),
	}:
		s.answerSent = true
	case <-s.ctx.Done():
		return fmt.Errorf("terminated")
	}

	return nil
}

func (s *webRTCSession) waitUntilConnected(pc *peerConnection) error {
	t := time.NewTimer(webrtcHandshakeTimeout)
	defer t.Stop()

outer:
	for {
		select {
		case <-t.C:
			return fmt.Errorf("deadline exceeded")

		case <-pc.connected:
			break outer

		case <-s.ctx.Done():
			return fmt.Errorf("terminated")
		}
	}

	s.pcMutex.Lock()
	s.pc = pc
	s.pcMutex.Unlock()

	return nil
}

func (s *webRTCSession) readRemoteCandidates(pc *peerConnection) {
	for {
		select {
		case req := <-s.chAddRemoteCandidates:
			for _, candidate := range req.candidates {
				err := pc.AddICECandidate(*candidate)
				if err != nil {
					req.res <- webRTCSessionAddCandidatesRes{err: err}
				}
			}
			req.res <- webRTCSessionAddCandidatesRes{}

		case <-s.ctx.Done():
			return
		}
	}
}

func (s *webRTCSession) addRemoteCandidates(
	req webRTCSessionAddCandidatesReq,
) webRTCSessionAddCandidatesRes {
	select {
	case s.chAddRemoteCandidates <- req:
		return <-req.res

	case <-s.ctx.Done():
		return webRTCSessionAddCandidatesRes{err: fmt.Errorf("terminated")}
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
	peerConnectionEstablished := false
	localCandidate := ""
	remoteCandidate := ""
	bytesReceived := uint64(0)
	bytesSent := uint64(0)

	pc := s.safePC()
	if pc != nil {
		peerConnectionEstablished = true
		localCandidate = pc.localCandidate()
		remoteCandidate = pc.remoteCandidate()
		bytesReceived = pc.bytesReceived()
		bytesSent = pc.bytesSent()
	}

	return &apiWebRTCSession{
		ID:                        s.uuid,
		Created:                   s.created,
		RemoteAddr:                s.req.remoteAddr,
		PeerConnectionEstablished: peerConnectionEstablished,
		LocalCandidate:            localCandidate,
		RemoteCandidate:           remoteCandidate,
		State: func() string {
			if s.req.publish {
				return "publish"
			}
			return "read"
		}(),
		BytesReceived: bytesReceived,
		BytesSent:     bytesSent,
	}
}
