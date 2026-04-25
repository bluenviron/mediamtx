package hls

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/hls"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	recreateInstancePause = 10 * time.Second
)

func emptyTimer() *time.Timer {
	t := time.NewTimer(0)
	<-t.C
	return t
}

type muxerCloseInstanceReq struct {
	instance *muxerInstance
	err      error
}

type muxer struct {
	parentCtx       context.Context
	remoteAddr      string
	variant         conf.HLSVariant
	segmentCount    int
	segmentDuration conf.Duration
	partDuration    conf.Duration
	segmentMaxSize  conf.StringSize
	directory       string
	closeAfter      conf.Duration
	wg              *sync.WaitGroup
	pathName        string
	pathManager     serverPathManager
	parent          *Server
	query           string

	ctx             context.Context
	ctxCancel       func()
	created         time.Time
	path            defs.Path
	lastRequestTime atomic.Int64
	bytesSent       atomic.Uint64

	mutex                            sync.RWMutex
	instance                         *muxerInstance
	cumulatedOutboundFramesDiscarded uint64
	sessionsBySecret                 map[uuid.UUID]*session

	chCloseInstance chan muxerCloseInstanceReq
}

func (m *muxer) initialize() {
	ctx, ctxCancel := context.WithCancel(m.parentCtx)

	m.ctx = ctx
	m.ctxCancel = ctxCancel
	m.created = time.Now()
	m.lastRequestTime.Store(time.Now().UnixNano())
	m.sessionsBySecret = make(map[uuid.UUID]*session)
	m.chCloseInstance = make(chan muxerCloseInstanceReq)

	m.Log(logger.Info, "created %s", func() string {
		if m.remoteAddr == "" {
			return "automatically"
		}
		return "(requested by " + m.remoteAddr + ")"
	}())

	// block first request to getInstance() until the first instance is available
	m.mutex.Lock()

	m.wg.Add(1)
	go m.run()
}

func (m *muxer) Close() {
	m.ctxCancel()
}

// Log implements logger.Writer.
func (m *muxer) Log(level logger.Level, format string, args ...any) {
	m.parent.Log(level, "[muxer %s] "+format, append([]any{m.pathName}, args...)...)
}

// PathName returns the path name.
func (m *muxer) PathName() string {
	return m.pathName
}

func (m *muxer) run() {
	defer m.wg.Done()

	err := m.runInner()

	m.ctxCancel()

	if m.instance != nil {
		m.instance.close()
	}

	m.mutex.Lock()

	m.instance = nil

	for _, sx := range m.sessionsBySecret {
		sx.close2(fmt.Errorf("muxer destroyed"))
	}

	m.mutex.Unlock()

	m.Log(logger.Info, "destroyed: %v", err)

	m.parent.closeMuxer(m)
}

func (m *muxer) runInner() error {
	res, err := m.pathManager.AddReader(defs.PathAddReaderReq{
		Author: m,
		AccessRequest: defs.PathAccessRequest{
			Name:     m.pathName,
			Query:    m.query,
			SkipAuth: true,
		},
	})
	if err != nil {
		m.mutex.Unlock()
		return err
	}

	m.path = res.Path

	defer m.path.RemoveReader(defs.PathRemoveReaderReq{Author: m})

	tmp, err := m.createInstance(res.Stream)
	if err != nil {
		if m.remoteAddr != "" || errors.Is(err, hls.ErrNoSupportedCodecs) {
			m.mutex.Unlock()
			return err
		}

		m.Log(logger.Error, "muxer instance crashed: %v", err)
	}

	m.instance = tmp
	m.mutex.Unlock()

	var recreateInstanceTimer *time.Timer

	if m.instance != nil {
		recreateInstanceTimer = emptyTimer()
	} else {
		recreateInstanceTimer = time.NewTimer(recreateInstancePause)
	}

	defer func() {
		recreateInstanceTimer.Stop()
	}()

	sessionCleanupTicker := time.NewTicker(sessionCleanupPeriod)
	defer sessionCleanupTicker.Stop()

	var activityCheckTimer *time.Timer
	if m.remoteAddr != "" {
		activityCheckTimer = time.NewTimer(max(time.Duration(m.closeAfter)/3, 1*time.Second))
	} else {
		activityCheckTimer = emptyTimer()
	}

	defer func() {
		activityCheckTimer.Stop()
	}()

	for {
		select {
		case req := <-m.chCloseInstance:
			if m.instance != req.instance {
				continue
			}

			m.mutex.Lock()
			m.cumulatedOutboundFramesDiscarded += m.instance.reader.OutboundFramesDiscarded()
			m.instance = nil
			m.mutex.Unlock()

			if m.remoteAddr != "" {
				return req.err
			} else {
				m.mutex.Lock()
				for _, sx := range m.sessionsBySecret {
					sx.close2(fmt.Errorf("muxer instance crashed"))
				}
				m.sessionsBySecret = make(map[uuid.UUID]*session)
				m.mutex.Unlock()

				m.Log(logger.Error, "muxer instance crashed: %v", req.err)
			}

			recreateInstanceTimer = time.NewTimer(recreateInstancePause)

		case <-recreateInstanceTimer.C:
			tmp, err = m.createInstance(res.Stream)
			if err != nil {
				m.Log(logger.Error, "muxer instance crashed: %v", err)
				recreateInstanceTimer = time.NewTimer(recreateInstancePause)
				continue
			}

			m.mutex.Lock()
			m.instance = tmp
			m.mutex.Unlock()

		case <-sessionCleanupTicker.C:
			now := time.Now()

			m.mutex.Lock()
			for secret, sx := range m.sessionsBySecret {
				lastRequest := time.Unix(0, sx.lastRequestTime.Load())

				if now.Sub(lastRequest) >= sessionCloseAfter {
					delete(m.sessionsBySecret, secret)
					sx.close2(fmt.Errorf("inactive"))
				}
			}
			m.mutex.Unlock()

		case <-activityCheckTimer.C:
			t := time.Unix(0, m.lastRequestTime.Load())
			if time.Since(t) >= time.Duration(m.closeAfter) {
				return fmt.Errorf("not used anymore")
			}
			activityCheckTimer = time.NewTimer(max(time.Duration(m.closeAfter)/3, 1*time.Second))

		case <-m.ctx.Done():
			return errors.New("terminated")
		}
	}
}

func (m *muxer) createInstance(strm *stream.Stream) (*muxerInstance, error) {
	mi := &muxerInstance{
		variant:         m.variant,
		segmentCount:    m.segmentCount,
		segmentDuration: m.segmentDuration,
		partDuration:    m.partDuration,
		segmentMaxSize:  m.segmentMaxSize,
		directory:       m.directory,
		pathName:        m.pathName,
		bytesSent:       &m.bytesSent,
		wg:              m.wg,
		stream:          strm,
		server:          m.parent,
		parent:          m,
	}
	err := mi.initialize()
	if err != nil {
		return nil, err
	}
	return mi, nil
}

func (m *muxer) closeInstance(mi *muxerInstance, err error) {
	select {
	case m.chCloseInstance <- muxerCloseInstanceReq{instance: mi, err: err}:
	case <-m.ctx.Done():
	}
}

// APIReaderDescribe implements reader.
func (m *muxer) APIReaderDescribe() *defs.APIPathReader {
	return &defs.APIPathReader{
		Type: defs.APIPathReaderTypeHidden,
		ID:   "",
	}
}

func (m *muxer) addSession(sx *session) ([]format.Format, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	select {
	case <-m.ctx.Done():
		return nil, fmt.Errorf("terminated")
	default:
	}

	if m.instance == nil {
		return nil, fmt.Errorf("muxer instance not available")
	}

	m.sessionsBySecret[sx.secret] = sx
	return m.instance.reader.Formats(), nil
}

func (m *muxer) findSession(ctx *gin.Context) *session {
	var rawSecret string
	if cookie, err := ctx.Request.Cookie(sessionCookieName); err == nil {
		rawSecret = cookie.Value
	} else {
		q := ctx.Request.URL.Query()
		rawSecret = q.Get(sessionQueryParamName)
	}

	secret, err := uuid.Parse(rawSecret)
	if err != nil {
		return nil
	}

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	sx, ok := m.sessionsBySecret[secret]
	if !ok {
		return nil
	}

	if ctx.ClientIP() != sx.ip {
		return nil
	}

	sx.lastRequestTime.Store(time.Now().UnixNano())

	return sx
}

func (m *muxer) handleRequest(ctx *gin.Context) error {
	m.lastRequestTime.Store(time.Now().UnixNano())

	m.mutex.RLock()
	instance := m.instance
	m.mutex.RUnlock()

	if instance == nil {
		return fmt.Errorf("muxer instance not available")
	}

	instance.handleRequest(ctx)
	return nil
}

func (m *muxer) apiItem() *defs.APIHLSMuxer {
	m.mutex.RLock()
	instance := m.instance
	cumulatedOutboundFramesDiscarded := m.cumulatedOutboundFramesDiscarded
	m.mutex.RUnlock()

	outboundFramesDiscarded := cumulatedOutboundFramesDiscarded
	if instance != nil {
		outboundFramesDiscarded += instance.reader.OutboundFramesDiscarded()
	}

	return &defs.APIHLSMuxer{
		Path:                    m.pathName,
		Created:                 m.created,
		LastRequest:             time.Unix(0, m.lastRequestTime.Load()),
		OutboundBytes:           m.bytesSent.Load(),
		OutboundFramesDiscarded: outboundFramesDiscarded,
		BytesSent:               m.bytesSent.Load(),
	}
}

func (m *muxer) apiSessionsList() []defs.APIHLSSession {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	sessions := make([]defs.APIHLSSession, 0, len(m.sessionsBySecret))

	for _, sx := range m.sessionsBySecret {
		sessions = append(sessions, *sx.apiItem())
	}

	return sessions
}

func (m *muxer) findSessionByUUID(uuid uuid.UUID) *session {
	for _, sx := range m.sessionsBySecret {
		if sx.uuid == uuid {
			return sx
		}
	}
	return nil
}

func (m *muxer) apiSessionsGet(uuid uuid.UUID) (*defs.APIHLSSession, bool) {
	m.mutex.RLock()
	sx := m.findSessionByUUID(uuid)
	if sx == nil {
		m.mutex.RUnlock()
		return nil, false
	}
	m.mutex.RUnlock()

	return sx.apiItem(), true
}

func (m *muxer) apiSessionsKick(uuid uuid.UUID) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	sx := m.findSessionByUUID(uuid)
	if sx == nil {
		return false
	}

	sx.close2(fmt.Errorf("kicked"))
	delete(m.sessionsBySecret, sx.secret)

	return true
}
