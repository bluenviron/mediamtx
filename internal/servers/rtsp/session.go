package rtsp

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"slices"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	rtspauth "github.com/bluenviron/gortsplib/v5/pkg/auth"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/headers"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/counterdumper"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/errordumper"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/hooks"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/rtsp"
	"github.com/bluenviron/mediamtx/internal/stream"
)

func profileLabel(p headers.TransportProfile) string {
	switch p {
	case headers.TransportProfileSAVP:
		return "SAVP"
	case headers.TransportProfileAVP:
		return "AVP"
	}
	return "unknown"
}

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
	subStream       *stream.SubStream
	onUnreadHook    func()
	packetsLost     *counterdumper.Dumper
	decodeErrors    *errordumper.Dumper
	discardedFrames *counterdumper.Dumper
}

func (s *session) initialize() {
	s.uuid = uuid.New()
	s.created = time.Now()

	s.packetsLost = &counterdumper.Dumper{
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

	s.decodeErrors = &errordumper.Dumper{
		OnReport: func(val uint64, last error) {
			if val == 1 {
				s.Log(logger.Warn, "decode error: %v", last)
			} else {
				s.Log(logger.Warn, "%d decode errors, last was: %v", val, last)
			}
		},
	}
	s.decodeErrors.Start()

	s.discardedFrames = &counterdumper.Dumper{
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
// this is not always called, so things that need to be released
// must go in onClose().
func (s *session) Close() {
	s.rsession.Close()
}

func (s *session) remoteAddr() net.Addr {
	return s.rconn.NetConn().RemoteAddr()
}

// Log implements logger.Writer.
func (s *session) Log(level logger.Level, format string, args ...any) {
	id := hex.EncodeToString(s.uuid[:4])
	s.parent.Log(level, "[session %s] "+format, append([]any{id}, args...)...)
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
	s.subStream = nil

	s.discardedFrames.Stop()
	s.decodeErrors.Stop()
	s.packetsLost.Stop()

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
	if slices.Contains(c.authMethods, rtspauth.VerifyMethodDigestMD5) {
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
		var terr *auth.Error
		if errors.As(err, &terr) {
			return c.handleAuthError(terr)
		}

		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, err
	}

	s.pathConf = pathConf

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

func (s *session) rtspStream() *gortsplib.ServerStream {
	if !s.isTLS {
		return s.stream.RTSPStream(s.rserver)
	}
	return s.stream.RTSPSStream(s.rserver)
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
	// we only have to handle the case in which the transport protocol is TCP
	// and it is disabled.
	if ctx.Transport.Protocol == gortsplib.ProtocolTCP {
		if _, ok := s.transports[gortsplib.ProtocolTCP]; !ok {
			return &base.Response{
				StatusCode: base.StatusUnsupportedTransport,
			}, nil, nil
		}
	}

	// CustomVerifyFunc prevents hashed credentials from working.
	// Use it only when strictly needed.
	var customVerifyFunc func(expectedUser, expectedPass string) bool
	if slices.Contains(c.authMethods, rtspauth.VerifyMethodDigestMD5) {
		customVerifyFunc = func(expectedUser, expectedPass string) bool {
			return c.rconn.VerifyCredentials(ctx.Request, expectedUser, expectedPass)
		}
	}

	switch s.rsession.State() {
	case gortsplib.ServerSessionStateInitial: // play
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
			var terr *auth.Error
			if errors.As(err, &terr) {
				res, err2 := c.handleAuthError(terr)
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

		return &base.Response{
			StatusCode: base.StatusOK,
		}, s.rtspStream(), nil

	case gortsplib.ServerSessionStatePrePlay: // play, subsequent calls
		return &base.Response{
			StatusCode: base.StatusOK,
		}, s.rtspStream(), nil

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
			s.rsession.Transport().Protocol,
			defs.MediasInfo(s.rsession.Medias()))

		s.onUnreadHook = hooks.OnRead(hooks.OnReadParams{
			Logger:          s,
			ExternalCmdPool: s.externalCmdPool,
			Conf:            s.path.SafeConf(),
			ExternalCmdEnv:  s.path.ExternalCmdEnv(),
			Reader:          *s.APIReaderDescribe(),
			Query:           s.rsession.Query(),
		})
	}

	return &base.Response{
		StatusCode: base.StatusOK,
		Header:     h,
	}, nil
}

// onRecord is called by rtspServer.
func (s *session) onRecord(_ *gortsplib.ServerHandlerOnRecordCtx) (*base.Response, error) {
	path, subStream, err := s.pathManager.AddPublisher(defs.PathAddPublisherReq{
		Author:        s,
		Desc:          s.rsession.AnnouncedDescription(),
		UseRTPPackets: true,
		ReplaceNTP:    !s.pathConf.UseAbsoluteTimestamp,
		ConfToCompare: s.pathConf,
		AccessRequest: defs.PathAccessRequest{
			Name:     s.rsession.Path()[1:],
			Query:    s.rsession.Query(),
			Publish:  true,
			SkipAuth: true,
		},
	})
	if err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, err
	}

	rtsp.ToStream(
		s.rsession,
		s.rsession.AnnouncedDescription().Medias,
		path.SafeConf(),
		&s.subStream,
		s)

	s.path = path
	s.subStream = subStream

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// onPause is called by rtspServer.
func (s *session) onPause(_ *gortsplib.ServerHandlerOnPauseCtx) (*base.Response, error) {
	switch s.rsession.State() {
	case gortsplib.ServerSessionStatePlay:
		s.onUnreadHook()

	case gortsplib.ServerSessionStateRecord:
		s.path.RemovePublisher(defs.PathRemovePublisherReq{Author: s})
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// APIReaderDescribe implements reader.
func (s *session) APIReaderDescribe() *defs.APIPathReader {
	return &defs.APIPathReader{
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
func (s *session) APISourceDescribe() *defs.APIPathSource {
	return &defs.APIPathSource{
		Type: func() string {
			if s.isTLS {
				return "rtspsSession"
			}
			return "rtspSession"
		}(),
		ID: s.uuid.String(),
	}
}

// onPacketLost is called by rtspServer.
func (s *session) onPacketsLost(ctx *gortsplib.ServerHandlerOnPacketsLostCtx) {
	s.packetsLost.Add(ctx.Lost)
}

// onDecodeError is called by rtspServer.
func (s *session) onDecodeError(ctx *gortsplib.ServerHandlerOnDecodeErrorCtx) {
	s.decodeErrors.Add(ctx.Error)
}

// onStreamWriteError is called by rtspServer.
func (s *session) onStreamWriteError(_ *gortsplib.ServerHandlerOnStreamWriteErrorCtx) {
	// currently the only error returned by OnStreamWriteError is ErrServerWriteQueueFull
	s.discardedFrames.Increase()
}

func (s *session) apiItem() *defs.APIRTSPSession {
	stats := s.rsession.Stats()

	return &defs.APIRTSPSession{
		ID:         s.uuid,
		Created:    s.created,
		RemoteAddr: s.remoteAddr().String(),
		State: func() defs.APIRTSPSessionState {
			state := s.rsession.State()
			switch state {
			case gortsplib.ServerSessionStatePlay:
				return defs.APIRTSPSessionStateRead

			case gortsplib.ServerSessionStateRecord:
				return defs.APIRTSPSessionStatePublish

			default:
				return defs.APIRTSPSessionStateIdle
			}
		}(),
		Path: func() string {
			pa := s.rsession.Path()
			if len(pa) >= 1 {
				return pa[1:]
			}
			return ""
		}(),
		Query: s.rsession.Query(),
		Transport: func() *string {
			transport := s.rsession.Transport()
			if transport == nil {
				return nil
			}
			v := transport.Protocol.String()
			return &v
		}(),
		Profile: func() *string {
			transport := s.rsession.Transport()
			if transport == nil {
				return nil
			}
			v := profileLabel(transport.Profile)
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
