package core

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/externalcmd"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

const (
	pauseAfterAuthError = 2 * time.Second
)

type rtspSessionPathManager interface {
	publisherAdd(req pathPublisherAddReq) pathPublisherAnnounceRes
	readerAdd(req pathReaderAddReq) pathReaderSetupPlayRes
}

type rtspSessionParent interface {
	log(logger.Level, string, ...interface{})
}

type rtspSession struct {
	isTLS           bool
	protocols       map[conf.Protocol]struct{}
	id              string
	ss              *gortsplib.ServerSession
	author          *gortsplib.ServerConn
	externalCmdPool *externalcmd.Pool
	pathManager     rtspSessionPathManager
	parent          rtspSessionParent

	created         time.Time
	path            *path
	stream          *stream
	state           gortsplib.ServerSessionState
	stateMutex      sync.Mutex
	onReadCmd       *externalcmd.Cmd // read
	announcedTracks gortsplib.Tracks // publish
}

func newRTSPSession(
	isTLS bool,
	protocols map[conf.Protocol]struct{},
	id string,
	ss *gortsplib.ServerSession,
	sc *gortsplib.ServerConn,
	externalCmdPool *externalcmd.Pool,
	pathManager rtspSessionPathManager,
	parent rtspSessionParent,
) *rtspSession {
	s := &rtspSession{
		isTLS:           isTLS,
		protocols:       protocols,
		id:              id,
		ss:              ss,
		author:          sc,
		externalCmdPool: externalCmdPool,
		pathManager:     pathManager,
		parent:          parent,
		created:         time.Now(),
	}

	s.log(logger.Info, "created by %v", s.author.NetConn().RemoteAddr())

	return s
}

// Close closes a Session.
func (s *rtspSession) close() {
	s.ss.Close()
}

// isRTSPSession implements pathRTSPSession.
func (s *rtspSession) isRTSPSession() {}

func (s *rtspSession) safeState() gortsplib.ServerSessionState {
	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()
	return s.state
}

func (s *rtspSession) remoteAddr() net.Addr {
	return s.author.NetConn().RemoteAddr()
}

func (s *rtspSession) log(level logger.Level, format string, args ...interface{}) {
	s.parent.log(level, "[session %s] "+format, append([]interface{}{s.id}, args...)...)
}

// onClose is called by rtspServer.
func (s *rtspSession) onClose(err error) {
	if s.ss.State() == gortsplib.ServerSessionStatePlay {
		if s.onReadCmd != nil {
			s.onReadCmd.Close()
			s.onReadCmd = nil
			s.log(logger.Info, "runOnRead command stopped")
		}
	}

	switch s.ss.State() {
	case gortsplib.ServerSessionStatePrePlay, gortsplib.ServerSessionStatePlay:
		s.path.readerRemove(pathReaderRemoveReq{author: s})

	case gortsplib.ServerSessionStatePreRecord, gortsplib.ServerSessionStateRecord:
		s.path.publisherRemove(pathPublisherRemoveReq{author: s})
	}

	s.path = nil
	s.stream = nil

	s.log(logger.Info, "destroyed (%v)", err)
}

// onAnnounce is called by rtspServer.
func (s *rtspSession) onAnnounce(c *rtspConn, ctx *gortsplib.ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	res := s.pathManager.publisherAdd(pathPublisherAddReq{
		author:   s,
		pathName: ctx.Path,
		authenticate: func(
			pathIPs []fmt.Stringer,
			pathUser conf.Credential,
			pathPass conf.Credential,
		) error {
			return c.authenticate(ctx.Path, pathIPs, pathUser, pathPass, true, ctx.Request, ctx.Query)
		},
	})

	if res.err != nil {
		switch terr := res.err.(type) {
		case pathErrAuthNotCritical:
			s.log(logger.Debug, "non-critical authentication error: %s", terr.message)
			return terr.response, nil

		case pathErrAuthCritical:
			// wait some seconds to stop brute force attacks
			<-time.After(pauseAfterAuthError)

			return terr.response, errors.New(terr.message)

		default:
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, res.err
		}
	}

	s.path = res.path
	s.announcedTracks = ctx.Tracks

	s.stateMutex.Lock()
	s.state = gortsplib.ServerSessionStatePreRecord
	s.stateMutex.Unlock()

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// onSetup is called by rtspServer.
func (s *rtspSession) onSetup(c *rtspConn, ctx *gortsplib.ServerHandlerOnSetupCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	// in case the client is setupping a stream with UDP or UDP-multicast, and these
	// transport protocols are disabled, gortsplib already blocks the request.
	// we have only to handle the case in which the transport protocol is TCP
	// and it is disabled.
	if ctx.Transport == gortsplib.TransportTCP {
		if _, ok := s.protocols[conf.Protocol(gortsplib.TransportTCP)]; !ok {
			return &base.Response{
				StatusCode: base.StatusUnsupportedTransport,
			}, nil, nil
		}
	}

	switch s.ss.State() {
	case gortsplib.ServerSessionStateInitial, gortsplib.ServerSessionStatePrePlay: // play
		res := s.pathManager.readerAdd(pathReaderAddReq{
			author:   s,
			pathName: ctx.Path,
			authenticate: func(
				pathIPs []fmt.Stringer,
				pathUser conf.Credential,
				pathPass conf.Credential,
			) error {
				return c.authenticate(ctx.Path, pathIPs, pathUser, pathPass, false, ctx.Request, ctx.Query)
			},
		})

		if res.err != nil {
			switch terr := res.err.(type) {
			case pathErrAuthNotCritical:
				s.log(logger.Debug, "non-critical authentication error: %s", terr.message)
				return terr.response, nil, nil

			case pathErrAuthCritical:
				// wait some seconds to stop brute force attacks
				<-time.After(pauseAfterAuthError)

				return terr.response, nil, errors.New(terr.message)

			case pathErrNoOnePublishing:
				return &base.Response{
					StatusCode: base.StatusNotFound,
				}, nil, res.err

			default:
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, nil, res.err
			}
		}

		s.path = res.path
		s.stream = res.stream

		if ctx.TrackID >= len(res.stream.tracks()) {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, nil, fmt.Errorf("track %d does not exist", ctx.TrackID)
		}

		s.stateMutex.Lock()
		s.state = gortsplib.ServerSessionStatePrePlay
		s.stateMutex.Unlock()

		return &base.Response{
			StatusCode: base.StatusOK,
		}, res.stream.rtspStream, nil

	default: // record
		return &base.Response{
			StatusCode: base.StatusOK,
		}, nil, nil
	}
}

// onPlay is called by rtspServer.
func (s *rtspSession) onPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	h := make(base.Header)

	if s.ss.State() == gortsplib.ServerSessionStatePrePlay {
		s.path.readerStart(pathReaderStartReq{author: s})

		tracks := make(gortsplib.Tracks, len(s.ss.SetuppedTracks()))
		n := 0
		for id := range s.ss.SetuppedTracks() {
			tracks[n] = s.stream.tracks()[id]
			n++
		}

		s.log(logger.Info, "is reading from path '%s', with %s, %s",
			s.path.Name(),
			s.ss.SetuppedTransport(),
			sourceTrackInfo(tracks))

		if s.path.Conf().RunOnRead != "" {
			s.log(logger.Info, "runOnRead command started")
			s.onReadCmd = externalcmd.NewCmd(
				s.externalCmdPool,
				s.path.Conf().RunOnRead,
				s.path.Conf().RunOnReadRestart,
				s.path.externalCmdEnv(),
				func(co int) {
					s.log(logger.Info, "runOnRead command exited with code %d", co)
				})
		}

		s.stateMutex.Lock()
		s.state = gortsplib.ServerSessionStatePlay
		s.stateMutex.Unlock()
	}

	return &base.Response{
		StatusCode: base.StatusOK,
		Header:     h,
	}, nil
}

// onRecord is called by rtspServer.
func (s *rtspSession) onRecord(ctx *gortsplib.ServerHandlerOnRecordCtx) (*base.Response, error) {
	res := s.path.publisherStart(pathPublisherStartReq{
		author:             s,
		tracks:             s.announcedTracks,
		generateRTPPackets: false,
	})
	if res.err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, res.err
	}

	s.log(logger.Info, "is publishing to path '%s', with %s, %s",
		s.path.Name(),
		s.ss.SetuppedTransport(),
		sourceTrackInfo(s.announcedTracks))

	s.stream = res.stream

	s.stateMutex.Lock()
	s.state = gortsplib.ServerSessionStateRecord
	s.stateMutex.Unlock()

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// onPause is called by rtspServer.
func (s *rtspSession) onPause(ctx *gortsplib.ServerHandlerOnPauseCtx) (*base.Response, error) {
	switch s.ss.State() {
	case gortsplib.ServerSessionStatePlay:
		if s.onReadCmd != nil {
			s.log(logger.Info, "runOnRead command stopped")
			s.onReadCmd.Close()
		}

		s.path.readerStop(pathReaderStopReq{author: s})

		s.stateMutex.Lock()
		s.state = gortsplib.ServerSessionStatePrePlay
		s.stateMutex.Unlock()

	case gortsplib.ServerSessionStateRecord:
		s.path.publisherStop(pathPublisherStopReq{author: s})

		s.stateMutex.Lock()
		s.state = gortsplib.ServerSessionStatePreRecord
		s.stateMutex.Unlock()
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// onReaderData implements reader.
func (s *rtspSession) onReaderData(data *data) {
	// packets are routed to the session by gortsplib.ServerStream.
}

// apiReaderDescribe implements reader.
func (s *rtspSession) apiReaderDescribe() interface{} {
	var typ string
	if s.isTLS {
		typ = "rtspsSession"
	} else {
		typ = "rtspSession"
	}

	return struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}{typ, s.id}
}

// apiSourceDescribe implements source.
func (s *rtspSession) apiSourceDescribe() interface{} {
	var typ string
	if s.isTLS {
		typ = "rtspsSession"
	} else {
		typ = "rtspSession"
	}

	return struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}{typ, s.id}
}

// onPacketRTP is called by rtspServer.
func (s *rtspSession) onPacketRTP(ctx *gortsplib.ServerHandlerOnPacketRTPCtx) {
	if ctx.H264NALUs != nil {
		s.stream.writeData(&data{
			trackID:      ctx.TrackID,
			rtpPacket:    ctx.Packet,
			ptsEqualsDTS: ctx.PTSEqualsDTS,
			pts:          ctx.H264PTS,
			h264NALUs:    ctx.H264NALUs,
		})
	} else {
		s.stream.writeData(&data{
			trackID:      ctx.TrackID,
			rtpPacket:    ctx.Packet,
			ptsEqualsDTS: ctx.PTSEqualsDTS,
		})
	}
}
