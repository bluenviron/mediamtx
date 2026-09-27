package hls

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/hooks"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

const (
	sessionCloseAfterInactivity  = 30 * time.Second
	sessionInactivityCheckPeriod = sessionCloseAfterInactivity / 3
)

type sessionServer interface {
	logger.Writer
	getOrCreateMuxer(string, *session, bool) (*muxer, error)
	closeSession(*session)
}

type session struct {
	wg              *sync.WaitGroup
	remoteAddr      string
	pathName        string
	query           string
	userAgent       string
	credentials     *auth.Credentials
	isCDN           bool
	externalCmdPool *externalcmd.Pool
	pathManager     serverPathManager
	server          sessionServer

	ctx             context.Context
	ctxCancel       func()
	ip              string
	uuid            uuid.UUID
	secret          uuid.UUID
	created         time.Time
	userMutex       sync.RWMutex
	user            string
	lastRequestTime atomic.Int64
	bytesSent       atomic.Uint64
	muxerInstance   *muxerInstance
	innerErr        error

	chReady chan struct{}
}

func (s *session) initialize() {
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	s.ip, _, _ = net.SplitHostPort(s.remoteAddr)
	s.uuid = uuid.New()
	s.secret = uuid.New()
	s.created = time.Now()
	s.lastRequestTime.Store(time.Now().UnixNano())
	s.chReady = make(chan struct{})

	if s.isCDN {
		s.Log(logger.Info, "created (CDN)")
	} else {
		s.Log(logger.Info, "created by %s", s.remoteAddr)
	}

	s.wg.Add(1)
	go s.run()
}

// called by path or path manager.
// not implemented since closing the Muxer instance is enough to close every associated session.
func (s *session) Close() {
}

func (s *session) close2() {
	s.ctxCancel()
}

// Log implements logger.Writer.
func (s *session) Log(level logger.Level, format string, args ...any) {
	id := hex.EncodeToString(s.uuid[:4])
	s.server.Log(level, "[session %v] "+format, append([]any{id}, args...)...)
}

func (s *session) run() {
	defer s.wg.Done()

	err := s.runInner()

	select {
	case <-s.chReady:
	default:
		s.innerErr = err
		close(s.chReady)
	}

	s.ctxCancel()

	s.server.closeSession(s)

	s.Log(logger.Info, "closed: %v", err)
}

func (s *session) runInner() error {
	accessReq := defs.PathAccessRequest{
		Name:                 s.pathName,
		Publish:              false,
		Proto:                auth.ProtocolHLS,
		ID:                   &s.uuid,
		EnableAskCredentials: true,
	}

	if s.isCDN {
		accessReq.SkipAuth = true
	} else {
		accessReq.Query = s.query
		accessReq.UserAgent = s.userAgent
		accessReq.Credentials = s.credentials
		accessReq.IP = net.ParseIP(s.ip)
	}

	res, err := s.pathManager.AddReader(defs.PathAddReaderReq{
		Author:        s,
		AccessRequest: accessReq,
	})
	if err != nil {
		return err
	}

	defer res.Path.RemoveReader(defs.PathRemoveReaderReq{Author: s})

	s.userMutex.Lock()
	s.user = res.User
	s.userMutex.Unlock()

	muxer, err := s.server.getOrCreateMuxer(
		s.pathName,
		s,
		res.Path.SafeConf().SourceOnDemand,
	)
	if err != nil {
		return err
	}

	muxerInstance := muxer.getInstance()
	if muxerInstance == nil {
		return fmt.Errorf("muxer instance not available")
	}

	s.muxerInstance = muxerInstance

	reader := &stream.Reader{
		Parent: s,
	}

	// this is needed to increase stream outbound bytes for every HLS session,
	// even if HLS sessions are not directly attached to streams (they are through muxers).
	for _, medi := range res.Stream.OrigDesc.Medias {
		for _, forma := range medi.Formats {
			if slices.Contains(muxerInstance.reader.Formats(), forma) {
				reader.OnData(medi, forma, func(_ *unit.Unit) error {
					return nil
				})
			}
		}
	}

	res.Stream.AddReader(reader)
	defer res.Stream.RemoveReader(reader)

	s.Log(logger.Info, "is reading from muxer '%s'", s.pathName)

	onUnreadHook := hooks.OnRead(hooks.OnReadParams{
		Logger:          s,
		ExternalCmdPool: s.externalCmdPool,
		Conf:            res.Path.SafeConf(),
		ExternalCmdEnv:  res.Path.ExternalCmdEnv(),
		Reader:          *s.APIReaderDescribe(),
		Query:           s.query,
	})
	defer onUnreadHook()

	close(s.chReady)

	activityCheckTimer := time.NewTimer(sessionInactivityCheckPeriod)
	defer func() {
		activityCheckTimer.Stop()
	}()

	for {
		select {
		case <-s.ctx.Done():
			return fmt.Errorf("terminated")

		case <-activityCheckTimer.C:
			if time.Since(time.Unix(0, s.lastRequestTime.Load())) > sessionCloseAfterInactivity {
				return fmt.Errorf("inactive")
			}
			activityCheckTimer = time.NewTimer(sessionInactivityCheckPeriod)

		case <-muxerInstance.ctx.Done():
			if s.isCDN {
				return fmt.Errorf("muxer instance closed")
			}

			absoluteEndTimer := time.NewTimer(sessionCloseAfterInactivity)
			defer absoluteEndTimer.Stop()

			for {
				select {
				case <-s.ctx.Done():
					return fmt.Errorf("terminated")

				case <-activityCheckTimer.C:
					if time.Since(time.Unix(0, s.lastRequestTime.Load())) > sessionCloseAfterInactivity {
						return fmt.Errorf("inactive")
					}

				case <-absoluteEndTimer.C:
					return fmt.Errorf("muxer instance closed")
				}
			}
		}
	}
}

func (s *session) handleRequest(ctx *gin.Context, q url.Values) error {
	s.lastRequestTime.Store(time.Now().UnixNano())

	ctx.Writer = &responseWriterCounter{
		ResponseWriter: ctx.Writer,
		bytesSent:      &s.bytesSent,
	}

	select {
	case <-s.chReady:
	case <-s.ctx.Done():
		select {
		case <-s.chReady:
		default:
			return fmt.Errorf("terminated")
		}
	}

	if s.innerErr != nil {
		return s.innerErr
	}

	if !s.isCDN {
		if cookie, err2 := ctx.Request.Cookie("cookieCheck"); err2 == nil && cookie.Value == "1" {
			// Use exclusively partitioned cookies for safety reasons.
			// Unfortunately they are available on HTTPS only. In case of HTTP, fall back to query parameters,
			// which are still not shared between different pages/domains but are visible in the URL.
			http.SetCookie(ctx.Writer, &http.Cookie{
				Name:        sessionCookieName,
				Value:       s.secret.String(),
				SameSite:    http.SameSiteNoneMode,
				Secure:      true,
				Partitioned: true,
				HttpOnly:    true,
			})
		} else {
			q.Set(sessionQueryParamName, s.secret.String())
			ctx.Request.URL.RawQuery = q.Encode()
		}
	}

	s.muxerInstance.handleRequest(ctx, s.isCDN)
	return nil
}

func (s *session) apiItem() *defs.APIHLSSession {
	s.userMutex.RLock()
	user := s.user
	s.userMutex.RUnlock()

	outboundBytes := s.bytesSent.Load()

	return &defs.APIHLSSession{
		ID:            s.uuid,
		Created:       s.created,
		RemoteAddr:    s.remoteAddr,
		Path:          s.pathName,
		Query:         s.query,
		User:          user,
		UserAgent:     s.userAgent,
		IsCDN:         s.isCDN,
		OutboundBytes: outboundBytes,
	}
}

func (s *session) APIReaderDescribe() *defs.APIPathReader {
	return &defs.APIPathReader{
		Type: defs.APIPathReaderTypeHLSSession,
		ID:   s.uuid.String(),
	}
}
