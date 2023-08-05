package core

import (
	"encoding/hex"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v3"
	"github.com/bluenviron/gortsplib/v3/pkg/auth"
	"github.com/bluenviron/gortsplib/v3/pkg/base"
	"github.com/bluenviron/gortsplib/v3/pkg/url"
	"github.com/google/uuid"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
)

type rtspSessionPathManager interface {
	addPublisher(req pathAddPublisherReq) pathAddPublisherRes
	addReader(req pathAddReaderReq) pathAddReaderRes
}

type rtspSessionParent interface {
	logger.Writer
}

type rtspSession struct {
	isTLS           bool
	protocols       map[conf.Protocol]struct{}
	session         *gortsplib.ServerSession
	author          *gortsplib.ServerConn
	externalCmdPool *externalcmd.Pool
	pathManager     rtspSessionPathManager
	parent          rtspSessionParent

	uuid      uuid.UUID
	created   time.Time
	path      *path
	stream    *stream.Stream
	onReadCmd *externalcmd.Cmd // read
	mutex     sync.Mutex
	state     gortsplib.ServerSessionState
	transport *gortsplib.Transport
	pathName  string
}

func newRTSPSession(
	isTLS bool,
	protocols map[conf.Protocol]struct{},
	session *gortsplib.ServerSession,
	sc *gortsplib.ServerConn,
	externalCmdPool *externalcmd.Pool,
	pathManager rtspSessionPathManager,
	parent rtspSessionParent,
) *rtspSession {
	s := &rtspSession{
		isTLS:           isTLS,
		protocols:       protocols,
		session:         session,
		author:          sc,
		externalCmdPool: externalCmdPool,
		pathManager:     pathManager,
		parent:          parent,
		uuid:            uuid.New(),
		created:         time.Now(),
	}

	s.Log(logger.Info, "created by %v", s.author.NetConn().RemoteAddr())

	return s
}

// Close closes a Session.
func (s *rtspSession) close() {
	s.session.Close()
}

func (s *rtspSession) remoteAddr() net.Addr {
	return s.author.NetConn().RemoteAddr()
}

func (s *rtspSession) Log(level logger.Level, format string, args ...interface{}) {
	id := hex.EncodeToString(s.uuid[:4])
	s.parent.Log(level, "[session %s] "+format, append([]interface{}{id}, args...)...)
}

// onClose is called by rtspServer.
func (s *rtspSession) onClose(err error) {
	if s.session.State() == gortsplib.ServerSessionStatePlay {
		if s.onReadCmd != nil {
			s.onReadCmd.Close()
			s.onReadCmd = nil
			s.Log(logger.Info, "runOnRead command stopped")
		}
	}

	switch s.session.State() {
	case gortsplib.ServerSessionStatePrePlay, gortsplib.ServerSessionStatePlay:
		s.path.removeReader(pathRemoveReaderReq{author: s})

	case gortsplib.ServerSessionStatePreRecord, gortsplib.ServerSessionStateRecord:
		s.path.removePublisher(pathRemovePublisherReq{author: s})
	}

	s.path = nil
	s.stream = nil

	s.Log(logger.Info, "destroyed (%v)", err)
}

// onAnnounce is called by rtspServer.
func (s *rtspSession) onAnnounce(c *rtspConn, ctx *gortsplib.ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	if len(ctx.Path) == 0 || ctx.Path[0] != '/' {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, fmt.Errorf("invalid path")
	}
	ctx.Path = ctx.Path[1:]

	if c.authNonce == "" {
		var err error
		c.authNonce, err = auth.GenerateNonce2()
		if err != nil {
			return &base.Response{
				StatusCode: base.StatusInternalServerError,
			}, err
		}
	}

	res := s.pathManager.addPublisher(pathAddPublisherReq{
		author:   s,
		pathName: ctx.Path,
		credentials: authCredentials{
			query:       ctx.Query,
			ip:          c.ip(),
			proto:       authProtocolRTSP,
			id:          &c.uuid,
			rtspRequest: ctx.Request,
			rtspBaseURL: nil,
			rtspNonce:   c.authNonce,
		},
	})

	if res.err != nil {
		switch terr := res.err.(type) {
		case *errAuthentication:
			return c.handleAuthError(terr)

		default:
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, res.err
		}
	}

	s.path = res.path

	s.mutex.Lock()
	s.state = gortsplib.ServerSessionStatePreRecord
	s.pathName = ctx.Path
	s.mutex.Unlock()

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// onSetup is called by rtspServer.
func (s *rtspSession) onSetup(c *rtspConn, ctx *gortsplib.ServerHandlerOnSetupCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	if len(ctx.Path) == 0 || ctx.Path[0] != '/' {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, nil, fmt.Errorf("invalid path")
	}
	ctx.Path = ctx.Path[1:]

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

	switch s.session.State() {
	case gortsplib.ServerSessionStateInitial, gortsplib.ServerSessionStatePrePlay: // play
		baseURL := &url.URL{
			Scheme:   ctx.Request.URL.Scheme,
			Host:     ctx.Request.URL.Host,
			Path:     ctx.Path,
			RawQuery: ctx.Query,
		}

		if ctx.Query != "" {
			baseURL.RawQuery += "/"
		} else {
			baseURL.Path += "/"
		}

		if c.authNonce == "" {
			var err error
			c.authNonce, err = auth.GenerateNonce2()
			if err != nil {
				return &base.Response{
					StatusCode: base.StatusInternalServerError,
				}, nil, err
			}
		}

		res := s.pathManager.addReader(pathAddReaderReq{
			author:   s,
			pathName: ctx.Path,
			credentials: authCredentials{
				query:       ctx.Query,
				ip:          c.ip(),
				proto:       authProtocolRTSP,
				id:          &c.uuid,
				rtspRequest: ctx.Request,
				rtspBaseURL: baseURL,
				rtspNonce:   c.authNonce,
			},
		})

		if res.err != nil {
			switch terr := res.err.(type) {
			case *errAuthentication:
				res, err := c.handleAuthError(terr)
				return res, nil, err

			case errPathNoOnePublishing:
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

		s.mutex.Lock()
		s.state = gortsplib.ServerSessionStatePrePlay
		s.pathName = ctx.Path
		s.mutex.Unlock()

		return &base.Response{
			StatusCode: base.StatusOK,
		}, res.stream.RTSPStream(), nil

	default: // record
		return &base.Response{
			StatusCode: base.StatusOK,
		}, nil, nil
	}
}

// onPlay is called by rtspServer.
func (s *rtspSession) onPlay(_ *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	h := make(base.Header)

	if s.session.State() == gortsplib.ServerSessionStatePrePlay {
		s.Log(logger.Info, "is reading from path '%s', with %s, %s",
			s.path.name,
			s.session.SetuppedTransport(),
			sourceMediaInfo(s.session.SetuppedMedias()))

		pathConf := s.path.safeConf()

		if pathConf.RunOnRead != "" {
			s.Log(logger.Info, "runOnRead command started")
			s.onReadCmd = externalcmd.NewCmd(
				s.externalCmdPool,
				pathConf.RunOnRead,
				pathConf.RunOnReadRestart,
				s.path.externalCmdEnv(),
				func(err error) {
					s.Log(logger.Info, "runOnRead command exited: %v", err)
				})
		}

		s.mutex.Lock()
		s.state = gortsplib.ServerSessionStatePlay
		s.transport = s.session.SetuppedTransport()
		s.mutex.Unlock()
	}

	return &base.Response{
		StatusCode: base.StatusOK,
		Header:     h,
	}, nil
}

// onRecord is called by rtspServer.
func (s *rtspSession) onRecord(_ *gortsplib.ServerHandlerOnRecordCtx) (*base.Response, error) {
	res := s.path.startPublisher(pathStartPublisherReq{
		author:             s,
		medias:             s.session.AnnouncedMedias(),
		generateRTPPackets: false,
	})
	if res.err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, res.err
	}

	s.stream = res.stream

	for _, medi := range s.session.AnnouncedMedias() {
		for _, forma := range medi.Formats {
			cmedi := medi
			cforma := forma

			s.session.OnPacketRTP(cmedi, cforma, func(pkt *rtp.Packet) {
				res.stream.WriteRTPPacket(cmedi, cforma, pkt, time.Now())
			})
		}
	}

	s.mutex.Lock()
	s.state = gortsplib.ServerSessionStateRecord
	s.transport = s.session.SetuppedTransport()
	s.mutex.Unlock()

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// onPause is called by rtspServer.
func (s *rtspSession) onPause(_ *gortsplib.ServerHandlerOnPauseCtx) (*base.Response, error) {
	switch s.session.State() {
	case gortsplib.ServerSessionStatePlay:
		if s.onReadCmd != nil {
			s.Log(logger.Info, "runOnRead command stopped")
			s.onReadCmd.Close()
		}

		s.mutex.Lock()
		s.state = gortsplib.ServerSessionStatePrePlay
		s.mutex.Unlock()

	case gortsplib.ServerSessionStateRecord:
		s.path.stopPublisher(pathStopPublisherReq{author: s})

		s.mutex.Lock()
		s.state = gortsplib.ServerSessionStatePreRecord
		s.mutex.Unlock()
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// apiReaderDescribe implements reader.
func (s *rtspSession) apiReaderDescribe() pathAPISourceOrReader {
	return pathAPISourceOrReader{
		Type: func() string {
			if s.isTLS {
				return "rtspsSession"
			}
			return "rtspSession"
		}(),
		ID: s.uuid.String(),
	}
}

// apiSourceDescribe implements source.
func (s *rtspSession) apiSourceDescribe() pathAPISourceOrReader {
	return s.apiReaderDescribe()
}

// onPacketLost is called by rtspServer.
func (s *rtspSession) onPacketLost(ctx *gortsplib.ServerHandlerOnPacketLostCtx) {
	s.Log(logger.Warn, ctx.Error.Error())
}

// onDecodeError is called by rtspServer.
func (s *rtspSession) onDecodeError(ctx *gortsplib.ServerHandlerOnDecodeErrorCtx) {
	s.Log(logger.Warn, ctx.Error.Error())
}

func (s *rtspSession) apiItem() *apiRTSPSession {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return &apiRTSPSession{
		ID:         s.uuid,
		Created:    s.created,
		RemoteAddr: s.remoteAddr().String(),
		State: func() apiRTSPSessionState {
			switch s.state {
			case gortsplib.ServerSessionStatePrePlay,
				gortsplib.ServerSessionStatePlay:
				return apiRTSPSessionStateRead

			case gortsplib.ServerSessionStatePreRecord,
				gortsplib.ServerSessionStateRecord:
				return apiRTSPSessionStatePublish
			}
			return apiRTSPSessionStateIdle
		}(),
		Path: s.pathName,
		Transport: func() *string {
			if s.transport == nil {
				return nil
			}
			v := s.transport.String()
			return &v
		}(),
		BytesReceived: s.session.BytesReceived(),
		BytesSent:     s.session.BytesSent(),
	}
}
