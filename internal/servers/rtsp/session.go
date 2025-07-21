package rtsp

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	rtspauth "github.com/bluenviron/gortsplib/v4/pkg/auth"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/counterdumper"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/hooks"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/rtsp"
	"github.com/bluenviron/mediamtx/internal/stream"
)

type session struct {
	isTLS           bool
	transports      conf.RTSPTransports
	rsession        *gortsplib.ServerSession
	rconn           *gortsplib.ServerConn
	rserver         *gortsplib.Server
	externalCmdPool *externalcmd.Pool
	pathManager     serverPathManager
	parent          logger.Writer

	uuid            uuid.UUID
	created         time.Time
	pathConf        *conf.Path // record only
	path            defs.Path
	stream          *stream.Stream
	onUnreadHook    func()
	mutex           sync.Mutex
	state           defs.APIRTSPSessionState
	transport       *gortsplib.Transport
	pathName        string
	query           string
	packetsLost     *counterdumper.CounterDumper
	decodeErrors    *counterdumper.CounterDumper
	discardedFrames *counterdumper.CounterDumper
}

func (s *session) initialize() {
	s.uuid = uuid.New()
	s.created = time.Now()
	s.state = defs.APIRTSPSessionStateIdle

	s.packetsLost = &counterdumper.CounterDumper{
		OnReport: func(val uint64) {
			s.Log(logger.Warn, "%d RTP %s lost",
				val,
				func() string {
					if val == 1 {
						return "packet"
					}
					return "packets"
				}())
		},
	}
	s.packetsLost.Start()

	s.decodeErrors = &counterdumper.CounterDumper{
		OnReport: func(val uint64) {
			s.Log(logger.Warn, "%d decode %s",
				val,
				func() string {
					if val == 1 {
						return "error"
					}
					return "errors"
				}())
		},
	}
	s.decodeErrors.Start()

	s.discardedFrames = &counterdumper.CounterDumper{
		OnReport: func(val uint64) {
			s.Log(logger.Warn, "reader is too slow, discarding %d %s",
				val,
				func() string {
					if val == 1 {
						return "frame"
					}
					return "frames"
				}())
		},
	}
	s.discardedFrames.Start()

	s.Log(logger.Info, "created by %v", s.rconn.NetConn().RemoteAddr())
}

// Close closes a Session.
func (s *session) Close() {
	s.discardedFrames.Stop()
	s.decodeErrors.Stop()
	s.packetsLost.Stop()
	s.rsession.Close()
}

func (s *session) remoteAddr() net.Addr {
	return s.rconn.NetConn().RemoteAddr()
}

// Log implements logger.Writer.
func (s *session) Log(level logger.Level, format string, args ...interface{}) {
	id := hex.EncodeToString(s.uuid[:4])
	s.parent.Log(level, "[session %s] "+format, append([]interface{}{id}, args...)...)
}

// onClose is called by rtspServer.
func (s *session) onClose(err error) {
	if s.rsession.State() == gortsplib.ServerSessionStatePlay {
		s.onUnreadHook()
	}

	switch s.rsession.State() {
	case gortsplib.ServerSessionStatePrePlay, gortsplib.ServerSessionStatePlay:
		s.path.RemoveReader(defs.PathRemoveReaderReq{Author: s})

	case gortsplib.ServerSessionStateRecord:
		s.path.RemovePublisher(defs.PathRemovePublisherReq{Author: s})
	}

	s.path = nil
	s.stream = nil

	s.Log(logger.Info, "destroyed: %v", err)
}

// onAnnounce is called by rtspServer.
func (s *session) onAnnounce(c *conn, ctx *gortsplib.ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	if len(ctx.Path) == 0 || ctx.Path[0] != '/' {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, fmt.Errorf("invalid path")
	}
	ctx.Path = ctx.Path[1:]

	// CustomVerifyFunc prevents hashed credentials from working.
	// Use it only when strictly needed.
	var customVerifyFunc func(expectedUser, expectedPass string) bool
	if contains(c.authMethods, rtspauth.VerifyMethodDigestMD5) {
		customVerifyFunc = func(expectedUser, expectedPass string) bool {
			return c.rconn.VerifyCredentials(ctx.Request, expectedUser, expectedPass)
		}
	}

	pathConf, err := s.pathManager.FindPathConf(defs.PathFindPathConfReq{
		AccessRequest: defs.PathAccessRequest{
			Name:             ctx.Path,
			Query:            ctx.Query,
			Publish:          true,
			Proto:            auth.ProtocolRTSP,
			ID:               &c.uuid,
			Credentials:      rtsp.Credentials(ctx.Request),
			IP:               c.ip(),
			CustomVerifyFunc: customVerifyFunc,
		},
	})
	if err != nil {
		var terr auth.Error
		if errors.As(err, &terr) {
			return c.handleAuthError(ctx.Request)
		}

		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, err
	}

	s.pathConf = pathConf

	s.mutex.Lock()
	s.pathName = ctx.Path
	s.query = ctx.Query
	s.mutex.Unlock()

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// onSetup is called by rtspServer.
func (s *session) onSetup(c *conn, ctx *gortsplib.ServerHandlerOnSetupCtx,
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
		if _, ok := s.transports[gortsplib.TransportTCP]; !ok {
			return &base.Response{
				StatusCode: base.StatusUnsupportedTransport,
			}, nil, nil
		}
	}

	// CustomVerifyFunc prevents hashed credentials from working.
	// Use it only when strictly needed.
	var customVerifyFunc func(expectedUser, expectedPass string) bool
	if contains(c.authMethods, rtspauth.VerifyMethodDigestMD5) {
		customVerifyFunc = func(expectedUser, expectedPass string) bool {
			return c.rconn.VerifyCredentials(ctx.Request, expectedUser, expectedPass)
		}
	}

	switch s.rsession.State() {
	case gortsplib.ServerSessionStateInitial, gortsplib.ServerSessionStatePrePlay: // play
		path, stream, err := s.pathManager.AddReader(defs.PathAddReaderReq{
			Author: s,
			AccessRequest: defs.PathAccessRequest{
				Name:             ctx.Path,
				Query:            ctx.Query,
				Proto:            auth.ProtocolRTSP,
				ID:               &c.uuid,
				Credentials:      rtsp.Credentials(ctx.Request),
				IP:               c.ip(),
				CustomVerifyFunc: customVerifyFunc,
			},
		})
		if err != nil {
			var terr auth.Error
			if errors.As(err, &terr) {
				res, err2 := c.handleAuthError(ctx.Request)
				return res, nil, err2
			}

			var terr2 defs.PathNoStreamAvailableError
			if errors.As(err, &terr2) {
				return &base.Response{
					StatusCode: base.StatusNotFound,
				}, nil, err
			}

			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, nil, err
		}

		s.path = path
		s.stream = stream

		s.mutex.Lock()
		s.pathName = ctx.Path
		s.query = ctx.Query
		s.mutex.Unlock()

		var rstream *gortsplib.ServerStream
		if !s.isTLS {
			rstream = stream.RTSPStream(s.rserver)
		} else {
			rstream = stream.RTSPSStream(s.rserver)
		}

		return &base.Response{
			StatusCode: base.StatusOK,
		}, rstream, nil

	default: // record
		return &base.Response{
			StatusCode: base.StatusOK,
		}, nil, nil
	}
}

// onPlay is called by rtspServer.
func (s *session) onPlay(_ *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	h := make(base.Header)

	if s.rsession.State() == gortsplib.ServerSessionStatePrePlay {
		s.Log(logger.Info, "is reading from path '%s', with %s, %s",
			s.path.Name(),
			s.rsession.SetuppedTransport(),
			defs.MediasInfo(s.rsession.SetuppedMedias()))

		s.onUnreadHook = hooks.OnRead(hooks.OnReadParams{
			Logger:          s,
			ExternalCmdPool: s.externalCmdPool,
			Conf:            s.path.SafeConf(),
			ExternalCmdEnv:  s.path.ExternalCmdEnv(),
			Reader:          s.APIReaderDescribe(),
			Query:           s.rsession.SetuppedQuery(),
		})

		s.mutex.Lock()
		s.state = defs.APIRTSPSessionStateRead
		s.transport = s.rsession.SetuppedTransport()
		s.mutex.Unlock()
	}

	return &base.Response{
		StatusCode: base.StatusOK,
		Header:     h,
	}, nil
}

// onRecord is called by rtspServer.
func (s *session) onRecord(_ *gortsplib.ServerHandlerOnRecordCtx) (*base.Response, error) {
	path, stream, err := s.pathManager.AddPublisher(defs.PathAddPublisherReq{
		Author:             s,
		Desc:               s.rsession.AnnouncedDescription(),
		GenerateRTPPackets: false,
		ConfToCompare:      s.pathConf,
		AccessRequest: defs.PathAccessRequest{
			Name:     s.pathName,
			Query:    s.query,
			Publish:  true,
			SkipAuth: true,
		},
	})
	if err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, err
	}

	s.path = path
	s.stream = stream

	rtsp.ToStream(
		s.rsession,
		s.rsession.AnnouncedDescription().Medias,
		s.path.SafeConf(),
		stream,
		s)

	s.mutex.Lock()
	s.state = defs.APIRTSPSessionStatePublish
	s.transport = s.rsession.SetuppedTransport()
	s.mutex.Unlock()

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// onPause is called by rtspServer.
func (s *session) onPause(_ *gortsplib.ServerHandlerOnPauseCtx) (*base.Response, error) {
	switch s.rsession.State() {
	case gortsplib.ServerSessionStatePlay:
		s.onUnreadHook()

		s.mutex.Lock()
		s.state = defs.APIRTSPSessionStateIdle
		s.mutex.Unlock()

	case gortsplib.ServerSessionStateRecord:
		s.path.RemovePublisher(defs.PathRemovePublisherReq{Author: s})

		s.mutex.Lock()
		s.state = defs.APIRTSPSessionStateIdle
		s.mutex.Unlock()
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// APIReaderDescribe implements reader.
func (s *session) APIReaderDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: func() string {
			if s.isTLS {
				return "rtspsSession"
			}
			return "rtspSession"
		}(),
		ID: s.uuid.String(),
	}
}

// APISourceDescribe implements source.
func (s *session) APISourceDescribe() defs.APIPathSourceOrReader {
	return s.APIReaderDescribe()
}

// onPacketLost is called by rtspServer.
func (s *session) onPacketsLost(ctx *gortsplib.ServerHandlerOnPacketsLostCtx) {
	s.packetsLost.Add(ctx.Lost)
}

// onDecodeError is called by rtspServer.
func (s *session) onDecodeError(_ *gortsplib.ServerHandlerOnDecodeErrorCtx) {
	s.decodeErrors.Increase()
}

// onStreamWriteError is called by rtspServer.
func (s *session) onStreamWriteError(_ *gortsplib.ServerHandlerOnStreamWriteErrorCtx) {
	// currently the only error returned by OnStreamWriteError is ErrServerWriteQueueFull
	s.discardedFrames.Increase()
}

func (s *session) apiItem() *defs.APIRTSPSession {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	stats := s.rsession.Stats()

	return &defs.APIRTSPSession{
		ID:         s.uuid,
		Created:    s.created,
		RemoteAddr: s.remoteAddr().String(),
		State:      s.state,
		Path:       s.pathName,
		Query:      s.query,
		Transport: func() *string {
			if s.transport == nil {
				return nil
			}
			v := s.transport.String()
			return &v
		}(),
		BytesReceived:       stats.BytesReceived,
		BytesSent:           stats.BytesSent,
		RTPPacketsReceived:  stats.RTPPacketsReceived,
		RTPPacketsSent:      stats.RTPPacketsSent,
		RTPPacketsLost:      stats.RTPPacketsLost,
		RTPPacketsInError:   stats.RTPPacketsInError,
		RTPPacketsJitter:    stats.RTPPacketsJitter,
		RTCPPacketsReceived: stats.RTCPPacketsReceived,
		RTCPPacketsSent:     stats.RTCPPacketsSent,
		RTCPPacketsInError:  stats.RTCPPacketsInError,
	}
}
