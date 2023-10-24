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

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpav1"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtph264"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpvp8"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpvp9"
	"github.com/bluenviron/gortsplib/v4/pkg/rtptime"
	"github.com/google/uuid"
	"github.com/pion/rtp"
	"github.com/pion/sdp/v3"
	pwebrtc "github.com/pion/webrtc/v3"

	"github.com/bluenviron/mediamtx/internal/asyncwriter"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/bluenviron/mediamtx/internal/webrtc"
)

type webrtcTrackWrapper struct {
	clockRate int
}

func (w webrtcTrackWrapper) ClockRate() int {
	return w.clockRate
}

func (webrtcTrackWrapper) PTSEqualsDTS(*rtp.Packet) bool {
	return true
}

type setupStreamFunc func(*webrtc.OutgoingTrack) error

func webrtcFindVideoTrack(
	stream *stream.Stream,
	writer *asyncwriter.Writer,
) (format.Format, setupStreamFunc) {
	var av1Format *format.AV1
	media := stream.Desc().FindFormat(&av1Format)

	if av1Format != nil {
		return av1Format, func(track *webrtc.OutgoingTrack) error {
			encoder := &rtpav1.Encoder{
				PayloadType:    105,
				PayloadMaxSize: webrtcPayloadMaxSize,
			}
			err := encoder.Init()
			if err != nil {
				return err
			}

			stream.AddReader(writer, media, av1Format, func(u unit.Unit) error {
				tunit := u.(*unit.AV1)

				if tunit.TU == nil {
					return nil
				}

				packets, err := encoder.Encode(tunit.TU)
				if err != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					pkt.Timestamp += tunit.RTPPackets[0].Timestamp
					track.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			})

			return nil
		}
	}

	var vp9Format *format.VP9
	media = stream.Desc().FindFormat(&vp9Format)

	if vp9Format != nil {
		return vp9Format, func(track *webrtc.OutgoingTrack) error {
			encoder := &rtpvp9.Encoder{
				PayloadType:    96,
				PayloadMaxSize: webrtcPayloadMaxSize,
			}
			err := encoder.Init()
			if err != nil {
				return err
			}

			stream.AddReader(writer, media, vp9Format, func(u unit.Unit) error {
				tunit := u.(*unit.VP9)

				if tunit.Frame == nil {
					return nil
				}

				packets, err := encoder.Encode(tunit.Frame)
				if err != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					pkt.Timestamp += tunit.RTPPackets[0].Timestamp
					track.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			})

			return nil
		}
	}

	var vp8Format *format.VP8
	media = stream.Desc().FindFormat(&vp8Format)

	if vp8Format != nil {
		return vp8Format, func(track *webrtc.OutgoingTrack) error {
			encoder := &rtpvp8.Encoder{
				PayloadType:    96,
				PayloadMaxSize: webrtcPayloadMaxSize,
			}
			err := encoder.Init()
			if err != nil {
				return err
			}

			stream.AddReader(writer, media, vp8Format, func(u unit.Unit) error {
				tunit := u.(*unit.VP8)

				if tunit.Frame == nil {
					return nil
				}

				packets, err := encoder.Encode(tunit.Frame)
				if err != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					pkt.Timestamp += tunit.RTPPackets[0].Timestamp
					track.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			})

			return nil
		}
	}

	var h264Format *format.H264
	media = stream.Desc().FindFormat(&h264Format)

	if h264Format != nil {
		return h264Format, func(track *webrtc.OutgoingTrack) error {
			encoder := &rtph264.Encoder{
				PayloadType:    96,
				PayloadMaxSize: webrtcPayloadMaxSize,
			}
			err := encoder.Init()
			if err != nil {
				return err
			}

			firstReceived := false
			var lastPTS time.Duration

			stream.AddReader(writer, media, h264Format, func(u unit.Unit) error {
				tunit := u.(*unit.H264)

				if tunit.AU == nil {
					return nil
				}

				if !firstReceived {
					firstReceived = true
				} else if tunit.PTS < lastPTS {
					return fmt.Errorf("WebRTC doesn't support H264 streams with B-frames")
				}
				lastPTS = tunit.PTS

				packets, err := encoder.Encode(tunit.AU)
				if err != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					pkt.Timestamp += tunit.RTPPackets[0].Timestamp
					track.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			})

			return nil
		}
	}

	return nil, nil
}

func webrtcFindAudioTrack(
	stream *stream.Stream,
	writer *asyncwriter.Writer,
) (format.Format, setupStreamFunc) {
	var opusFormat *format.Opus
	media := stream.Desc().FindFormat(&opusFormat)

	if opusFormat != nil {
		return opusFormat, func(track *webrtc.OutgoingTrack) error {
			stream.AddReader(writer, media, opusFormat, func(u unit.Unit) error {
				for _, pkt := range u.GetRTPPackets() {
					track.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			})
			return nil
		}
	}

	var g722Format *format.G722
	media = stream.Desc().FindFormat(&g722Format)

	if g722Format != nil {
		return g722Format, func(track *webrtc.OutgoingTrack) error {
			stream.AddReader(writer, media, g722Format, func(u unit.Unit) error {
				for _, pkt := range u.GetRTPPackets() {
					track.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			})
			return nil
		}
	}

	var g711Format *format.G711
	media = stream.Desc().FindFormat(&g711Format)

	if g711Format != nil {
		return g711Format, func(track *webrtc.OutgoingTrack) error {
			stream.AddReader(writer, media, g711Format, func(u unit.Unit) error {
				for _, pkt := range u.GetRTPPackets() {
					track.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			})
			return nil
		}
	}

	return nil, nil
}

func webrtcMediasOfIncomingTracks(tracks []*webrtc.IncomingTrack) []*description.Media {
	ret := make([]*description.Media, len(tracks))

	for i, track := range tracks {
		forma := track.Format()

		var mediaType description.MediaType

		switch forma.(type) {
		case *format.AV1, *format.VP9, *format.VP8, *format.H264:
			mediaType = description.MediaTypeVideo

		default:
			mediaType = description.MediaTypeAudio
		}

		ret[i] = &description.Media{
			Type:    mediaType,
			Formats: []format.Format{forma},
		}
	}

	return ret
}

func whipOffer(body []byte) *pwebrtc.SessionDescription {
	return &pwebrtc.SessionDescription{
		Type: pwebrtc.SDPTypeOffer,
		SDP:  string(body),
	}
}

type webRTCSessionPathManager interface {
	addPublisher(req pathAddPublisherReq) pathAddPublisherRes
	addReader(req pathAddReaderReq) pathAddReaderRes
}

type webRTCSession struct {
	writeQueueSize  int
	api             *pwebrtc.API
	req             webRTCNewSessionReq
	wg              *sync.WaitGroup
	externalCmdPool *externalcmd.Pool
	pathManager     webRTCSessionPathManager
	parent          *webRTCManager

	ctx       context.Context
	ctxCancel func()
	created   time.Time
	uuid      uuid.UUID
	secret    uuid.UUID
	mutex     sync.RWMutex
	pc        *webrtc.PeerConnection

	chNew           chan webRTCNewSessionReq
	chAddCandidates chan webRTCAddSessionCandidatesReq
}

func newWebRTCSession(
	parentCtx context.Context,
	writeQueueSize int,
	api *pwebrtc.API,
	req webRTCNewSessionReq,
	wg *sync.WaitGroup,
	externalCmdPool *externalcmd.Pool,
	pathManager webRTCSessionPathManager,
	parent *webRTCManager,
) *webRTCSession {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	s := &webRTCSession{
		writeQueueSize:  writeQueueSize,
		api:             api,
		req:             req,
		wg:              wg,
		externalCmdPool: externalCmdPool,
		pathManager:     pathManager,
		parent:          parent,
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

	s.Log(logger.Info, "closed: %v", err)
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
		author: s,
		accessRequest: pathAccessRequest{
			name:    s.req.pathName,
			query:   s.req.query,
			publish: true,
			ip:      net.ParseIP(ip),
			user:    s.req.user,
			pass:    s.req.pass,
			proto:   authProtocolWebRTC,
			id:      &s.uuid,
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

	iceServers, err := s.parent.generateICEServers()
	if err != nil {
		return http.StatusInternalServerError, err
	}

	pc := &webrtc.PeerConnection{
		ICEServers: iceServers,
		API:        s.api,
		Publish:    false,
		Log:        s,
	}
	err = pc.Start()
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

	trackCount, err := webrtc.TrackCount(sdp.MediaDescriptions)
	if err != nil {
		return http.StatusBadRequest, err
	}

	answer, err := pc.CreateFullAnswer(s.ctx, offer)
	if err != nil {
		return http.StatusBadRequest, err
	}

	s.writeAnswer(answer)

	go s.readRemoteCandidates(pc)

	err = pc.WaitUntilConnected(s.ctx)
	if err != nil {
		return 0, err
	}

	s.mutex.Lock()
	s.pc = pc
	s.mutex.Unlock()

	tracks, err := pc.GatherIncomingTracks(s.ctx, trackCount)
	if err != nil {
		return 0, err
	}

	medias := webrtcMediasOfIncomingTracks(tracks)

	rres := res.path.startPublisher(pathStartPublisherReq{
		author:             s,
		desc:               &description.Session{Medias: medias},
		generateRTPPackets: false,
	})
	if rres.err != nil {
		return 0, rres.err
	}

	timeDecoder := rtptime.NewGlobalDecoder()

	for i, media := range medias {
		ci := i
		cmedia := media
		trackWrapper := &webrtcTrackWrapper{clockRate: cmedia.Formats[0].ClockRate()}

		go func() {
			for {
				pkt, err := tracks[ci].ReadRTP()
				if err != nil {
					return
				}

				pts, ok := timeDecoder.Decode(trackWrapper, pkt)
				if !ok {
					continue
				}

				rres.stream.WriteRTPPacket(cmedia, cmedia.Formats[0], pkt, time.Now(), pts)
			}
		}()
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
		author: s,
		accessRequest: pathAccessRequest{
			name:  s.req.pathName,
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

	iceServers, err := s.parent.generateICEServers()
	if err != nil {
		return http.StatusInternalServerError, err
	}

	pc := &webrtc.PeerConnection{
		ICEServers: iceServers,
		API:        s.api,
		Publish:    false,
		Log:        s,
	}
	err = pc.Start()
	if err != nil {
		return http.StatusBadRequest, err
	}
	defer pc.Close()

	writer := asyncwriter.New(s.writeQueueSize, s)

	videoTrack, videoSetup := webrtcFindVideoTrack(res.stream, writer)
	audioTrack, audioSetup := webrtcFindAudioTrack(res.stream, writer)

	if videoTrack == nil && audioTrack == nil {
		return http.StatusBadRequest, fmt.Errorf(
			"the stream doesn't contain any supported codec, which are currently AV1, VP9, VP8, H264, Opus, G722, G711")
	}

	tracks, err := pc.SetupOutgoingTracks(videoTrack, audioTrack)
	if err != nil {
		return http.StatusBadRequest, err
	}

	offer := whipOffer(s.req.offer)

	answer, err := pc.CreateFullAnswer(s.ctx, offer)
	if err != nil {
		return http.StatusBadRequest, err
	}

	s.writeAnswer(answer)

	go s.readRemoteCandidates(pc)

	err = pc.WaitUntilConnected(s.ctx)
	if err != nil {
		return 0, err
	}

	s.mutex.Lock()
	s.pc = pc
	s.mutex.Unlock()

	defer res.stream.RemoveReader(writer)

	n := 0

	if videoTrack != nil {
		err := videoSetup(tracks[n])
		if err != nil {
			return 0, err
		}
		n++
	}

	if audioTrack != nil {
		err := audioSetup(tracks[n])
		if err != nil {
			return 0, err
		}
	}

	s.Log(logger.Info, "is reading from path '%s', %s",
		res.path.name, readerMediaInfo(writer, res.stream))

	pathConf := res.path.safeConf()

	onUnreadHook := readerOnReadHook(
		s.externalCmdPool,
		pathConf,
		res.path,
		s.apiReaderDescribe(),
		s.req.query,
		s,
	)
	defer onUnreadHook()

	writer.Start()

	select {
	case <-pc.Disconnected():
		writer.Stop()
		return 0, fmt.Errorf("peer connection closed")

	case err := <-writer.Error():
		return 0, err

	case <-s.ctx.Done():
		writer.Stop()
		return 0, fmt.Errorf("terminated")
	}
}

func (s *webRTCSession) writeAnswer(answer *pwebrtc.SessionDescription) {
	s.req.res <- webRTCNewSessionRes{
		sx:     s,
		answer: []byte(answer.SDP),
	}
}

func (s *webRTCSession) readRemoteCandidates(pc *webrtc.PeerConnection) {
	for {
		select {
		case req := <-s.chAddCandidates:
			for _, candidate := range req.candidates {
				err := pc.AddRemoteCandidate(*candidate)
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
func (s *webRTCSession) apiSourceDescribe() apiPathSourceOrReader {
	return apiPathSourceOrReader{
		Type: "webRTCSession",
		ID:   s.uuid.String(),
	}
}

// apiReaderDescribe implements reader.
func (s *webRTCSession) apiReaderDescribe() apiPathSourceOrReader {
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
