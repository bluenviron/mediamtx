package core

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/bluenviron/gortsplib/v3/pkg/ringbuffer"
	"github.com/google/uuid"
	"github.com/pion/ice/v2"
	"github.com/pion/webrtc/v3"

	"github.com/aler9/mediamtx/internal/logger"
)

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
	cr *webRTCCandidateReader,
	trackRecv chan trackRecvPair,
) ([]*webRTCIncomingTrack, error) {
	var tracks []*webRTCIncomingTrack

	t := time.NewTimer(webrtcTrackGatherTimeout)
	defer t.Stop()

	for {
		select {
		case <-t.C:
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

		case err := <-cr.readError:
			return nil, fmt.Errorf("websocket error: %v", err)

		case <-ctx.Done():
			return nil, fmt.Errorf("terminated")
		}
	}
}

type webRTCSessionWHEPPathManager interface {
	readerAdd(req pathReaderAddReq) pathReaderSetupPlayRes
}

type webRTCSessionWHEP struct {
	readBufferCount   int
	req               webRTCNewSessionWHEPReq
	wg                *sync.WaitGroup
	iceHostNAT1To1IPs []string
	iceUDPMux         ice.UDPMux
	iceTCPMux         ice.TCPMux
	pathManager       webRTCSessionWHEPPathManager
	parent            *webRTCServer

	ctx        context.Context
	ctxCancel  func()
	uuid       uuid.UUID
	secret     uuid.UUID
	answerSent bool

	chAddRemoteCandidates chan webRTCSessionWHEPRemoteCandidatesReq
}

func newWebRTCSessionWHEP(
	parentCtx context.Context,
	readBufferCount int,
	req webRTCNewSessionWHEPReq,
	wg *sync.WaitGroup,
	iceHostNAT1To1IPs []string,
	iceUDPMux ice.UDPMux,
	iceTCPMux ice.TCPMux,
	pathManager webRTCSessionWHEPPathManager,
	parent *webRTCServer,
) *webRTCSessionWHEP {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	s := &webRTCSessionWHEP{
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
		uuid:                  uuid.New(),
		secret:                uuid.New(),
		chAddRemoteCandidates: make(chan webRTCSessionWHEPRemoteCandidatesReq),
	}

	s.Log(logger.Info, "created by %s", req.remoteAddr)

	wg.Add(1)
	go s.run()

	return s
}

func (s *webRTCSessionWHEP) Log(level logger.Level, format string, args ...interface{}) {
	id := hex.EncodeToString(s.uuid[:4])
	s.parent.Log(level, "[WHEP session %v] "+format, append([]interface{}{id}, args...)...)
}

func (s *webRTCSessionWHEP) close() {
	s.ctxCancel()
}

func (s *webRTCSessionWHEP) run() {
	defer s.wg.Done()

	err := s.runInner()

	if !s.answerSent {
		select {
		case s.req.res <- webRTCNewSessionWHEPRes{
			err: err,
		}:
		case <-s.ctx.Done():
		}
	}

	s.parent.sessionWHEPClose(s)

	s.Log(logger.Info, "closed (%v)", err)
}

func (s *webRTCSessionWHEP) runInner() error {
	res := s.pathManager.readerAdd(pathReaderAddReq{
		author:   s,
		pathName: s.req.pathName,
		skipAuth: true,
	})
	if res.err != nil {
		return res.err
	}

	defer res.path.readerRemove(pathReaderRemoveReq{author: s})

	tracks, err := gatherOutgoingTracks(res.stream.medias())
	if err != nil {
		return err
	}

	offer, err := s.decodeOffer()
	if err != nil {
		return err
	}

	pc, err := newPeerConnection(
		"",
		"",
		s.parent.genICEServers(),
		s.iceHostNAT1To1IPs,
		s.iceUDPMux,
		s.iceTCPMux,
		s)
	if err != nil {
		return err
	}
	defer pc.close()

	for _, track := range tracks {
		var err error
		track.sender, err = pc.AddTrack(track.track)
		if err != nil {
			return err
		}
	}

	err = pc.SetRemoteDescription(*offer)
	if err != nil {
		return err
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return err
	}

	err = pc.SetLocalDescription(answer)
	if err != nil {
		return err
	}

	err = s.waitGatheringDone(pc)
	if err != nil {
		return err
	}

	err = s.sendAnswer(pc.LocalDescription())
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case req := <-s.chAddRemoteCandidates:
				for _, candidate := range req.candidates {
					err := pc.AddICECandidate(*candidate)
					if err != nil {
						req.res <- webRTCSessionWHEPRemoteCandidatesRes{err: err}
					}
				}
				req.res <- webRTCSessionWHEPRemoteCandidatesRes{}

			case <-s.ctx.Done():
				return
			}
		}
	}()

	err = s.waitUntilConnected(pc)
	if err != nil {
		return err
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
		return fmt.Errorf("peer connection closed")

	case err := <-writeError:
		return err

	case <-s.ctx.Done():
		return fmt.Errorf("terminated")
	}
}

func (s *webRTCSessionWHEP) decodeOffer() (*webrtc.SessionDescription, error) {
	var offer webrtc.SessionDescription
	err := json.Unmarshal(s.req.offer, &offer)
	if err != nil {
		return nil, err
	}

	if offer.Type != webrtc.SDPTypeOffer {
		return nil, fmt.Errorf("received SDP is not an offer")
	}

	return &offer, nil
}

func (s *webRTCSessionWHEP) waitGatheringDone(pc *peerConnection) error {
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

func (s *webRTCSessionWHEP) sendAnswer(answer *webrtc.SessionDescription) error {
	enc, err := json.Marshal(answer)
	if err != nil {
		return err
	}

	select {
	case s.req.res <- webRTCNewSessionWHEPRes{
		sx:     s,
		answer: enc,
	}:
		s.answerSent = true
	case <-s.ctx.Done():
		return fmt.Errorf("terminated")
	}

	return nil
}

func (s *webRTCSessionWHEP) waitUntilConnected(pc *peerConnection) error {
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

	return nil
}

func (s *webRTCSessionWHEP) addRemoteCandidates(
	req webRTCSessionWHEPRemoteCandidatesReq,
) webRTCSessionWHEPRemoteCandidatesRes {
	select {
	case s.chAddRemoteCandidates <- req:
		return <-req.res

	case <-s.ctx.Done():
		return webRTCSessionWHEPRemoteCandidatesRes{err: fmt.Errorf("terminated")}
	}
}

// apiReaderDescribe implements reader.
func (s *webRTCSessionWHEP) apiReaderDescribe() pathAPISourceOrReader {
	return pathAPISourceOrReader{
		Type: "webRTCSessionWHEP",
		ID:   s.uuid.String(),
	}
}
