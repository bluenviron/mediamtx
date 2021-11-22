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
	onPublisherAnnounce(req pathPublisherAnnounceReq) pathPublisherAnnounceRes
	onReaderSetupPlay(req pathReaderSetupPlayReq) pathReaderSetupPlayRes
}

type rtspSessionParent interface {
	log(logger.Level, string, ...interface{})
}

type rtspSession struct {
	isTLS       bool
	rtspAddress string
	protocols   map[conf.Protocol]struct{}
	id          string
	ss          *gortsplib.ServerSession
	author      *gortsplib.ServerConn
	pathManager rtspSessionPathManager
	parent      rtspSessionParent

	path            *path
	state           gortsplib.ServerSessionState
	stateMutex      sync.Mutex
	setuppedTracks  map[int]*gortsplib.Track // read
	onReadCmd       *externalcmd.Cmd         // read
	announcedTracks gortsplib.Tracks         // publish
	stream          *stream                  // publish
}

func newRTSPSession(
	isTLS bool,
	rtspAddress string,
	protocols map[conf.Protocol]struct{},
	id string,
	ss *gortsplib.ServerSession,
	sc *gortsplib.ServerConn,
	pathManager rtspSessionPathManager,
	parent rtspSessionParent) *rtspSession {
	s := &rtspSession{
		isTLS:       isTLS,
		rtspAddress: rtspAddress,
		protocols:   protocols,
		id:          id,
		ss:          ss,
		author:      sc,
		pathManager: pathManager,
		parent:      parent,
	}

	s.log(logger.Info, "opened by %v", s.author.NetConn().RemoteAddr())

	return s
}

// Close closes a Session.
func (s *rtspSession) close() {
	s.ss.Close()
}

// IsRTSPSession implements pathRTSPSession.
func (s *rtspSession) IsRTSPSession() {}

// ID returns the public ID of the session.
func (s *rtspSession) ID() string {
	return s.id
}

func (s *rtspSession) safeState() gortsplib.ServerSessionState {
	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()
	return s.state
}

// RemoteAddr returns the remote address of the author of the session.
func (s *rtspSession) RemoteAddr() net.Addr {
	return s.author.NetConn().RemoteAddr()
}

func (s *rtspSession) log(level logger.Level, format string, args ...interface{}) {
	s.parent.log(level, "[session %s] "+format, append([]interface{}{s.id}, args...)...)
}

// onClose is called by rtspServer.
func (s *rtspSession) onClose(err error) {
	if s.ss.State() == gortsplib.ServerSessionStateRead {
		if s.onReadCmd != nil {
			s.onReadCmd.Close()
			s.onReadCmd = nil
			s.log(logger.Info, "runOnRead command stopped")
		}
	}

	switch s.ss.State() {
	case gortsplib.ServerSessionStatePreRead, gortsplib.ServerSessionStateRead:
		s.path.onReaderRemove(pathReaderRemoveReq{Author: s})
		s.path = nil

	case gortsplib.ServerSessionStatePrePublish, gortsplib.ServerSessionStatePublish:
		s.path.onPublisherRemove(pathPublisherRemoveReq{Author: s})
		s.path = nil
	}

	s.log(logger.Info, "closed (%v)", err)
}

// onAnnounce is called by rtspServer.
func (s *rtspSession) onAnnounce(c *rtspConn, ctx *gortsplib.ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	for i, track := range ctx.Tracks {
		if track.IsH264() {
			_, err := track.ExtractConfigH264()
			if err != nil {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("track %d is not valid", i+1)
			}
		}

		if track.IsAAC() {
			_, err := track.ExtractConfigAAC()
			if err != nil {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("track %d is not valid", i+1)
			}
		}

		if track.IsOpus() {
			_, err := track.ExtractConfigOpus()
			if err != nil {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("track %d is not valid", i+1)
			}
		}
	}

	res := s.pathManager.onPublisherAnnounce(pathPublisherAnnounceReq{
		Author:   s,
		PathName: ctx.Path,
		IP:       ctx.Conn.NetConn().RemoteAddr().(*net.TCPAddr).IP,
		ValidateCredentials: func(pathUser conf.Credential, pathPass conf.Credential) error {
			return c.validateCredentials(pathUser, pathPass, ctx.Req)
		},
	})

	if res.Err != nil {
		switch terr := res.Err.(type) {
		case pathErrAuthNotCritical:
			return terr.Response, nil

		case pathErrAuthCritical:
			// wait some seconds to stop brute force attacks
			<-time.After(pauseAfterAuthError)

			return terr.Response, errors.New(terr.Message)

		default:
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, res.Err
		}
	}

	s.path = res.Path
	s.announcedTracks = ctx.Tracks

	s.stateMutex.Lock()
	s.state = gortsplib.ServerSessionStatePrePublish
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
	case gortsplib.ServerSessionStateInitial, gortsplib.ServerSessionStatePreRead: // play
		res := s.pathManager.onReaderSetupPlay(pathReaderSetupPlayReq{
			Author:   s,
			PathName: ctx.Path,
			IP:       ctx.Conn.NetConn().RemoteAddr().(*net.TCPAddr).IP,
			ValidateCredentials: func(pathUser conf.Credential, pathPass conf.Credential) error {
				return c.validateCredentials(pathUser, pathPass, ctx.Req)
			},
		})

		if res.Err != nil {
			switch terr := res.Err.(type) {
			case pathErrAuthNotCritical:
				return terr.Response, nil, nil

			case pathErrAuthCritical:
				// wait some seconds to stop brute force attacks
				<-time.After(pauseAfterAuthError)

				return terr.Response, nil, errors.New(terr.Message)

			case pathErrNoOnePublishing:
				return &base.Response{
					StatusCode: base.StatusNotFound,
				}, nil, res.Err

			default:
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, nil, res.Err
			}
		}

		s.path = res.Path

		if ctx.TrackID >= len(res.Stream.tracks()) {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, nil, fmt.Errorf("track %d does not exist", ctx.TrackID)
		}

		if s.setuppedTracks == nil {
			s.setuppedTracks = make(map[int]*gortsplib.Track)
		}
		s.setuppedTracks[ctx.TrackID] = res.Stream.tracks()[ctx.TrackID]

		s.stateMutex.Lock()
		s.state = gortsplib.ServerSessionStatePreRead
		s.stateMutex.Unlock()

		return &base.Response{
			StatusCode: base.StatusOK,
		}, res.Stream.rtspStream, nil

	default: // record
		return &base.Response{
			StatusCode: base.StatusOK,
		}, nil, nil
	}
}

// onPlay is called by rtspServer.
func (s *rtspSession) onPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	h := make(base.Header)

	if s.ss.State() == gortsplib.ServerSessionStatePreRead {
		s.path.onReaderPlay(pathReaderPlayReq{Author: s})

		if s.path.Conf().RunOnRead != "" {
			s.log(logger.Info, "runOnRead command started")
			_, port, _ := net.SplitHostPort(s.rtspAddress)
			s.onReadCmd = externalcmd.New(s.path.Conf().RunOnRead, s.path.Conf().RunOnReadRestart, externalcmd.Environment{
				Path: s.path.Name(),
				Port: port,
			})
		}

		s.stateMutex.Lock()
		s.state = gortsplib.ServerSessionStateRead
		s.stateMutex.Unlock()
	}

	return &base.Response{
		StatusCode: base.StatusOK,
		Header:     h,
	}, nil
}

// onRecord is called by rtspServer.
func (s *rtspSession) onRecord(ctx *gortsplib.ServerHandlerOnRecordCtx) (*base.Response, error) {
	res := s.path.onPublisherRecord(pathPublisherRecordReq{
		Author: s,
		Tracks: s.announcedTracks,
	})
	if res.Err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, res.Err
	}

	s.stream = res.Stream

	s.stateMutex.Lock()
	s.state = gortsplib.ServerSessionStatePublish
	s.stateMutex.Unlock()

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// onPause is called by rtspServer.
func (s *rtspSession) onPause(ctx *gortsplib.ServerHandlerOnPauseCtx) (*base.Response, error) {
	switch s.ss.State() {
	case gortsplib.ServerSessionStateRead:
		if s.onReadCmd != nil {
			s.log(logger.Info, "runOnRead command stopped")
			s.onReadCmd.Close()
		}

		s.path.onReaderPause(pathReaderPauseReq{Author: s})

		s.stateMutex.Lock()
		s.state = gortsplib.ServerSessionStatePreRead
		s.stateMutex.Unlock()

	case gortsplib.ServerSessionStatePublish:
		s.path.onPublisherPause(pathPublisherPauseReq{Author: s})

		s.stateMutex.Lock()
		s.state = gortsplib.ServerSessionStatePrePublish
		s.stateMutex.Unlock()
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// onReaderAccepted implements reader.
func (s *rtspSession) onReaderAccepted() {
	tracksLen := len(s.ss.SetuppedTracks())

	s.log(logger.Info, "is reading from path '%s', %d %s with %s",
		s.path.Name(),
		tracksLen,
		func() string {
			if tracksLen == 1 {
				return "track"
			}
			return "tracks"
		}(),
		s.ss.SetuppedTransport())
}

// onReaderPacketRTP implements reader.
func (s *rtspSession) onReaderPacketRTP(trackID int, payload []byte) {
	s.ss.WritePacketRTP(trackID, payload)
}

// onReaderPacketRTCP implements reader.
func (s *rtspSession) onReaderPacketRTCP(trackID int, payload []byte) {
	s.ss.WritePacketRTCP(trackID, payload)
}

// onReaderAPIDescribe implements reader.
func (s *rtspSession) onReaderAPIDescribe() interface{} {
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

// onSourceAPIDescribe implements source.
func (s *rtspSession) onSourceAPIDescribe() interface{} {
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

// onPublisherAccepted implements publisher.
func (s *rtspSession) onPublisherAccepted(tracksLen int) {
	s.log(logger.Info, "is publishing to path '%s', %d %s with %s",
		s.path.Name(),
		tracksLen,
		func() string {
			if tracksLen == 1 {
				return "track"
			}
			return "tracks"
		}(),
		s.ss.SetuppedTransport())
}

// onPacketRTP is called by rtspServer.
func (s *rtspSession) onPacketRTP(ctx *gortsplib.ServerHandlerOnPacketRTPCtx) {
	if s.ss.State() != gortsplib.ServerSessionStatePublish {
		return
	}

	s.stream.onPacketRTP(ctx.TrackID, ctx.Payload)
}

// onPacketRTCP is called by rtspServer.
func (s *rtspSession) onPacketRTCP(ctx *gortsplib.ServerHandlerOnPacketRTCPCtx) {
	if s.ss.State() != gortsplib.ServerSessionStatePublish {
		return
	}

	s.stream.onPacketRTCP(ctx.TrackID, ctx.Payload)
}
