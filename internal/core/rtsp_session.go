package core

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/headers"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/externalcmd"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

const (
	pauseAfterAuthError = 2 * time.Second
)

type rtspSessionParent interface {
	Log(logger.Level, string, ...interface{})
}

type rtspSession struct {
	rtspAddress string
	protocols   map[conf.Protocol]struct{}
	visualID    string
	ss          *gortsplib.ServerSession
	pathManager *pathManager
	parent      rtspSessionParent

	path           *path
	setuppedTracks map[int]*gortsplib.Track // read
	onReadCmd      *externalcmd.Cmd         // read
}

func newRTSPSession(
	rtspAddress string,
	protocols map[conf.Protocol]struct{},
	visualID string,
	ss *gortsplib.ServerSession,
	sc *gortsplib.ServerConn,
	pathManager *pathManager,
	parent rtspSessionParent) *rtspSession {
	s := &rtspSession{
		rtspAddress: rtspAddress,
		protocols:   protocols,
		visualID:    visualID,
		ss:          ss,
		pathManager: pathManager,
		parent:      parent,
	}

	s.log(logger.Info, "opened by %v", sc.NetConn().RemoteAddr())

	return s
}

// ParentClose closes a Session.
func (s *rtspSession) ParentClose() {
	if s.ss.State() == gortsplib.ServerSessionStatePlay {
		if s.onReadCmd != nil {
			s.onReadCmd.Close()
		}
	}

	switch s.ss.State() {
	case gortsplib.ServerSessionStatePrePlay, gortsplib.ServerSessionStatePlay:
		s.path.OnReaderRemove(pathReaderRemoveReq{Author: s})
		s.path = nil

	case gortsplib.ServerSessionStatePreRecord, gortsplib.ServerSessionStateRecord:
		s.path.OnPublisherRemove(pathPublisherRemoveReq{Author: s})
		s.path = nil
	}

	s.log(logger.Info, "closed")
}

// Close closes a Session.
func (s *rtspSession) Close() {
	s.ss.Close()
}

// IsSource implements source.
func (s *rtspSession) IsSource() {}

// IsRTSPSession implements pathRTSPSession.
func (s *rtspSession) IsRTSPSession() {}

// VisualID returns the visual ID of the session.
func (s *rtspSession) VisualID() string {
	return s.visualID
}

func (s *rtspSession) displayedProtocol() string {
	if *s.ss.SetuppedDelivery() == base.StreamDeliveryMulticast {
		return "UDP-multicast"
	}
	return s.ss.SetuppedProtocol().String()
}

func (s *rtspSession) log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[session %s] "+format, append([]interface{}{s.visualID}, args...)...)
}

// OnAnnounce is called by rtspServer.
func (s *rtspSession) OnAnnounce(c *rtspConn, ctx *gortsplib.ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	res := s.pathManager.OnPublisherAnnounce(pathPublisherAnnounceReq{
		Author:   s,
		PathName: ctx.Path,
		Tracks:   ctx.Tracks,
		IP:       ctx.Conn.NetConn().RemoteAddr().(*net.TCPAddr).IP,
		ValidateCredentials: func(authMethods []headers.AuthMethod, pathUser string, pathPass string) error {
			return c.validateCredentials(authMethods, pathUser, pathPass, ctx.Path, ctx.Req)
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

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// OnSetup is called by rtspServer.
func (s *rtspSession) OnSetup(c *rtspConn, ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	if ctx.Transport.Protocol == base.StreamProtocolUDP {
		if _, ok := s.protocols[conf.ProtocolUDP]; !ok {
			return &base.Response{
				StatusCode: base.StatusUnsupportedTransport,
			}, nil, nil
		}

		if ctx.Transport.Delivery != nil && *ctx.Transport.Delivery == base.StreamDeliveryMulticast {
			if _, ok := s.protocols[conf.ProtocolMulticast]; !ok {
				return &base.Response{
					StatusCode: base.StatusUnsupportedTransport,
				}, nil, nil
			}
		}
	} else if _, ok := s.protocols[conf.ProtocolTCP]; !ok {
		return &base.Response{
			StatusCode: base.StatusUnsupportedTransport,
		}, nil, nil
	}

	switch s.ss.State() {
	case gortsplib.ServerSessionStateInitial, gortsplib.ServerSessionStatePrePlay: // play
		res := s.pathManager.OnReaderSetupPlay(pathReaderSetupPlayReq{
			Author:   s,
			PathName: ctx.Path,
			IP:       ctx.Conn.NetConn().RemoteAddr().(*net.TCPAddr).IP,
			ValidateCredentials: func(authMethods []headers.AuthMethod, pathUser string, pathPass string) error {
				return c.validateCredentials(authMethods, pathUser, pathPass, ctx.Path, ctx.Req)
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

		if ctx.TrackID >= len(res.Stream.Tracks()) {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, nil, fmt.Errorf("track %d does not exist", ctx.TrackID)
		}

		if s.setuppedTracks == nil {
			s.setuppedTracks = make(map[int]*gortsplib.Track)
		}
		s.setuppedTracks[ctx.TrackID] = res.Stream.Tracks()[ctx.TrackID]

		return &base.Response{
			StatusCode: base.StatusOK,
		}, res.Stream, nil

	default: // record
		return &base.Response{
			StatusCode: base.StatusOK,
		}, nil, nil
	}
}

// OnPlay is called by rtspServer.
func (s *rtspSession) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	h := make(base.Header)

	if s.ss.State() == gortsplib.ServerSessionStatePrePlay {
		if ctx.Path != s.path.Name() {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, fmt.Errorf("path has changed, was '%s', now is '%s'", s.path.Name(), ctx.Path)
		}

		s.path.OnReaderPlay(pathReaderPlayReq{Author: s})

		if s.path.Conf().RunOnRead != "" {
			_, port, _ := net.SplitHostPort(s.rtspAddress)
			s.onReadCmd = externalcmd.New(s.path.Conf().RunOnRead, s.path.Conf().RunOnReadRestart, externalcmd.Environment{
				Path: s.path.Name(),
				Port: port,
			})
		}
	}

	return &base.Response{
		StatusCode: base.StatusOK,
		Header:     h,
	}, nil
}

// OnRecord is called by rtspServer.
func (s *rtspSession) OnRecord(ctx *gortsplib.ServerHandlerOnRecordCtx) (*base.Response, error) {
	if ctx.Path != s.path.Name() {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, fmt.Errorf("path has changed, was '%s', now is '%s'", s.path.Name(), ctx.Path)
	}

	res := s.path.OnPublisherRecord(pathPublisherRecordReq{Author: s})
	if res.Err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, res.Err
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// OnPause is called by rtspServer.
func (s *rtspSession) OnPause(ctx *gortsplib.ServerHandlerOnPauseCtx) (*base.Response, error) {
	switch s.ss.State() {
	case gortsplib.ServerSessionStatePlay:
		if s.onReadCmd != nil {
			s.onReadCmd.Close()
		}

		s.path.OnReaderPause(pathReaderPauseReq{Author: s})

	case gortsplib.ServerSessionStateRecord:
		s.path.OnPublisherPause(pathPublisherPauseReq{Author: s})
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// OnReaderAccepted implements reader.
func (s *rtspSession) OnReaderAccepted() {
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
		s.displayedProtocol())
}

// OnReaderFrame implements reader.
func (s *rtspSession) OnReaderFrame(trackID int, streamType gortsplib.StreamType, payload []byte) {
	s.ss.WriteFrame(trackID, streamType, payload)
}

// OnPublisherAccepted implements publisher.
func (s *rtspSession) OnPublisherAccepted(tracksLen int) {
	s.log(logger.Info, "is publishing to path '%s', %d %s with %s",
		s.path.Name(),
		tracksLen,
		func() string {
			if tracksLen == 1 {
				return "track"
			}
			return "tracks"
		}(),
		s.displayedProtocol())
}

// OnIncomingFrame is called by rtspServer.
func (s *rtspSession) OnIncomingFrame(ctx *gortsplib.ServerHandlerOnFrameCtx) {
	if s.ss.State() != gortsplib.ServerSessionStateRecord {
		return
	}

	s.path.OnSourceFrame(ctx.TrackID, ctx.StreamType, ctx.Payload)
}
