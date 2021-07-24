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

type rtspSessionPathMan interface {
	OnReadPublisherSetupPlay(readPublisherSetupPlayReq)
	OnReadPublisherAnnounce(readPublisherAnnounceReq)
}

type rtspSessionParent interface {
	Log(logger.Level, string, ...interface{})
}

type rtspSession struct {
	rtspAddress string
	protocols   map[conf.Protocol]struct{}
	visualID    string
	ss          *gortsplib.ServerSession
	pathMan     rtspSessionPathMan
	parent      rtspSessionParent

	path           readPublisherPath
	setuppedTracks map[int]*gortsplib.Track // read
	onReadCmd      *externalcmd.Cmd         // read
	onPublishCmd   *externalcmd.Cmd         // publish
}

func newRTSPSession(
	rtspAddress string,
	protocols map[conf.Protocol]struct{},
	visualID string,
	ss *gortsplib.ServerSession,
	sc *gortsplib.ServerConn,
	pathMan rtspSessionPathMan,
	parent rtspSessionParent) *rtspSession {
	s := &rtspSession{
		rtspAddress: rtspAddress,
		protocols:   protocols,
		visualID:    visualID,
		ss:          ss,
		pathMan:     pathMan,
		parent:      parent,
	}

	s.log(logger.Info, "opened by %v", sc.NetConn().RemoteAddr())

	return s
}

// ParentClose closes a Session.
func (s *rtspSession) ParentClose() {
	switch s.ss.State() {
	case gortsplib.ServerSessionStatePlay:
		if s.onReadCmd != nil {
			s.onReadCmd.Close()
		}

	case gortsplib.ServerSessionStateRecord:
		if s.onPublishCmd != nil {
			s.onPublishCmd.Close()
		}
	}

	if s.path != nil {
		res := make(chan struct{})
		s.path.OnReadPublisherRemove(readPublisherRemoveReq{s, res}) //nolint:govet
		<-res
		s.path = nil
	}

	s.log(logger.Info, "closed")
}

// Close closes a Session.
func (s *rtspSession) Close() {
	s.ss.Close()
}

// IsReadPublisher implements readPublisher.
func (s *rtspSession) IsReadPublisher() {}

// IsSource implements source.
func (s *rtspSession) IsSource() {}

// IsRTSPSession implements path.rtspSession.
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

// OnAnnounce is called by rtspserver.Server.
func (s *rtspSession) OnAnnounce(c *rtspConn, ctx *gortsplib.ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	resc := make(chan readPublisherAnnounceRes)
	s.pathMan.OnReadPublisherAnnounce(readPublisherAnnounceReq{
		Author:   s,
		PathName: ctx.Path,
		Tracks:   ctx.Tracks,
		IP:       ctx.Conn.NetConn().RemoteAddr().(*net.TCPAddr).IP,
		ValidateCredentials: func(authMethods []headers.AuthMethod, pathUser string, pathPass string) error {
			return c.ValidateCredentials(authMethods, pathUser, pathPass, ctx.Path, ctx.Req)
		},
		Res: resc,
	})
	res := <-resc

	if res.Err != nil {
		switch terr := res.Err.(type) {
		case readPublisherErrAuthNotCritical:
			return terr.Response, nil

		case readPublisherErrAuthCritical:
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

// OnSetup is called by rtspserver.Server.
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
		resc := make(chan readPublisherSetupPlayRes)
		s.pathMan.OnReadPublisherSetupPlay(readPublisherSetupPlayReq{
			Author:   s,
			PathName: ctx.Path,
			IP:       ctx.Conn.NetConn().RemoteAddr().(*net.TCPAddr).IP,
			ValidateCredentials: func(authMethods []headers.AuthMethod, pathUser string, pathPass string) error {
				return c.ValidateCredentials(authMethods, pathUser, pathPass, ctx.Path, ctx.Req)
			},
			Res: resc,
		})
		res := <-resc

		if res.Err != nil {
			switch terr := res.Err.(type) {
			case readPublisherErrAuthNotCritical:
				return terr.Response, nil, nil

			case readPublisherErrAuthCritical:
				// wait some seconds to stop brute force attacks
				<-time.After(pauseAfterAuthError)

				return terr.Response, nil, errors.New(terr.Message)

			case readPublisherErrNoOnePublishing:
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

// OnPlay is called by rtspserver.Server.
func (s *rtspSession) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	h := make(base.Header)

	if s.ss.State() == gortsplib.ServerSessionStatePrePlay {
		if ctx.Path != s.path.Name() {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, fmt.Errorf("path has changed, was '%s', now is '%s'", s.path.Name(), ctx.Path)
		}

		resc := make(chan readPublisherPlayRes)
		s.path.OnReadPublisherPlay(readPublisherPlayReq{s, resc}) //nolint:govet
		<-resc

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

// OnRecord is called by rtspserver.Server.
func (s *rtspSession) OnRecord(ctx *gortsplib.ServerHandlerOnRecordCtx) (*base.Response, error) {
	if ctx.Path != s.path.Name() {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, fmt.Errorf("path has changed, was '%s', now is '%s'", s.path.Name(), ctx.Path)
	}

	resc := make(chan readPublisherRecordRes)
	s.path.OnReadPublisherRecord(readPublisherRecordReq{Author: s, Res: resc})
	res := <-resc

	if res.Err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, res.Err
	}

	tracksLen := len(s.ss.AnnouncedTracks())

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

	if s.path.Conf().RunOnPublish != "" {
		_, port, _ := net.SplitHostPort(s.rtspAddress)
		s.onPublishCmd = externalcmd.New(s.path.Conf().RunOnPublish, s.path.Conf().RunOnPublishRestart, externalcmd.Environment{
			Path: s.path.Name(),
			Port: port,
		})
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// OnPause is called by rtspserver.Server.
func (s *rtspSession) OnPause(ctx *gortsplib.ServerHandlerOnPauseCtx) (*base.Response, error) {
	switch s.ss.State() {
	case gortsplib.ServerSessionStatePlay:
		if s.onReadCmd != nil {
			s.onReadCmd.Close()
		}

		res := make(chan struct{})
		s.path.OnReadPublisherPause(readPublisherPauseReq{s, res}) //nolint:govet
		<-res

	case gortsplib.ServerSessionStateRecord:
		if s.onPublishCmd != nil {
			s.onPublishCmd.Close()
		}

		res := make(chan struct{})
		s.path.OnReadPublisherPause(readPublisherPauseReq{s, res}) //nolint:govet
		<-res
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// OnFrame implements path.Reader.
func (s *rtspSession) OnFrame(trackID int, streamType gortsplib.StreamType, payload []byte) {
	s.ss.WriteFrame(trackID, streamType, payload)
}

// OnIncomingFrame is called by rtspserver.Server.
func (s *rtspSession) OnIncomingFrame(ctx *gortsplib.ServerHandlerOnFrameCtx) {
	if s.ss.State() != gortsplib.ServerSessionStateRecord {
		return
	}

	s.path.OnFrame(ctx.TrackID, ctx.StreamType, ctx.Payload)
}
