// Package hls contains a HLS server.
package hls

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
)

// ErrMuxerNotFound is returned when a muxer is not found.
var ErrMuxerNotFound = errors.New("muxer not found")

// ErrSessionNotFound is returned when a session is not found.
var ErrSessionNotFound = errors.New("session not found")

func interfaceIsEmpty(i any) bool {
	return reflect.ValueOf(i).Kind() != reflect.Pointer || reflect.ValueOf(i).IsNil()
}

func sessionGetSecret(ctx *gin.Context) *uuid.UUID {
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

	return &secret
}

type serverMetrics interface {
	SetHLSServer(defs.APIHLSServer)
}

type serverPathManager interface {
	SetHLSServer(*Server) []defs.Path
	FindPathConf(req defs.PathFindPathConfReq) (*defs.PathFindPathConfRes, error)
	AddReader(req defs.PathAddReaderReq) (*defs.PathAddReaderRes, error)
}

type serverParent interface {
	logger.Writer
}

// Server is a HLS server.
type Server struct {
	Address         string
	DumpPackets     bool
	Encryption      bool
	ServerKey       string
	ServerCert      string
	AllowOrigins    []string
	TrustedProxies  conf.IPNetworks
	AlwaysRemux     bool
	Variant         conf.HLSVariant
	SegmentCount    int
	SegmentDuration conf.Duration
	PartDuration    conf.Duration
	SegmentMaxSize  conf.StringSize
	Directory       string
	CDNSecret       string
	ReadTimeout     conf.Duration
	WriteTimeout    conf.Duration
	MuxerCloseAfter conf.Duration
	ExternalCmdPool *externalcmd.Pool
	Metrics         serverMetrics
	PathManager     serverPathManager
	Parent          serverParent

	closed     atomic.Bool
	wg         sync.WaitGroup
	httpServer *httpServer

	muxersMutex sync.RWMutex
	muxers      map[string]*muxer

	sessionsMutex     sync.RWMutex
	sessionsBySecret  map[uuid.UUID]*session
	cdnSessionsByPath map[string]*session
}

// Initialize initializes the server.
func (s *Server) Initialize() error {
	s.muxers = make(map[string]*muxer)
	s.sessionsBySecret = make(map[uuid.UUID]*session)
	s.cdnSessionsByPath = make(map[string]*session)

	s.httpServer = &httpServer{
		address:        s.Address,
		dumpPackets:    s.DumpPackets,
		encryption:     s.Encryption,
		serverKey:      s.ServerKey,
		serverCert:     s.ServerCert,
		allowOrigins:   s.AllowOrigins,
		trustedProxies: s.TrustedProxies,
		readTimeout:    s.ReadTimeout,
		writeTimeout:   s.WriteTimeout,
		cdnSecret:      s.CDNSecret,
		pathManager:    s.PathManager,
		parent:         s,
	}
	err := s.httpServer.initialize()
	if err != nil {
		return err
	}

	str := "started with listener on " + s.Address
	if !s.Encryption {
		str += " (TCP/HTTP)"
	} else {
		str += " (TCP/HTTPS)"
	}
	s.Log(logger.Info, str)

	s.muxersMutex.Lock()
	readyPaths := s.PathManager.SetHLSServer(s)
	for _, pa := range readyPaths {
		s.createAutomaticMuxerLocked(pa)
	}
	s.muxersMutex.Unlock()

	if !interfaceIsEmpty(s.Metrics) {
		s.Metrics.SetHLSServer(s)
	}

	return nil
}

// Log implements logger.Writer.
func (s *Server) Log(level logger.Level, format string, args ...any) {
	s.Parent.Log(level, "[HLS] "+format, args...)
}

// Close closes the server.
func (s *Server) Close() {
	s.Log(logger.Info, "closing")

	if !interfaceIsEmpty(s.Metrics) {
		s.Metrics.SetHLSServer(nil)
	}

	s.closed.Store(true)

	s.muxersMutex.Lock()
	s.sessionsMutex.Lock()

	muxers := make([]*muxer, 0, len(s.muxers))
	for _, mx := range s.muxers {
		muxers = append(muxers, mx)
	}
	clear(s.muxers)

	sessions := make([]*session, 0, len(s.sessionsBySecret)+len(s.cdnSessionsByPath))
	for _, sx := range s.sessionsBySecret {
		sessions = append(sessions, sx)
	}
	for _, sx := range s.cdnSessionsByPath {
		sessions = append(sessions, sx)
	}
	clear(s.sessionsBySecret)
	clear(s.cdnSessionsByPath)

	s.sessionsMutex.Unlock()
	s.muxersMutex.Unlock()

	s.PathManager.SetHLSServer(nil)

	for _, sx := range sessions {
		sx.close2()
	}
	for _, mx := range muxers {
		mx.Close()
	}

	s.httpServer.close()
	s.wg.Wait()

	s.Log(logger.Debug, "closed")
}

func (s *Server) createAutomaticMuxerLocked(pa defs.Path) {
	if s.AlwaysRemux && !pa.SafeConf().SourceOnDemand {
		if _, ok := s.muxers[pa.Name()]; !ok {
			s.createMuxerLocked(pa.Name(), nil)
		}
	}
}

func (s *Server) createMuxerLocked(pathName string, author *session) *muxer {
	r := &muxer{
		variant:         s.Variant,
		segmentCount:    s.SegmentCount,
		segmentDuration: s.SegmentDuration,
		partDuration:    s.PartDuration,
		segmentMaxSize:  s.SegmentMaxSize,
		directory:       s.Directory,
		wg:              &s.wg,
		pathName:        pathName,
		pathManager:     s.PathManager,
		parent:          s,
		author:          author,
		closeAfter:      s.MuxerCloseAfter,
	}
	r.initialize()
	s.muxers[pathName] = r
	return r
}

// closeMuxer is called by muxer.
func (s *Server) closeMuxer(mx *muxer) {
	s.muxersMutex.Lock()
	if current, ok := s.muxers[mx.PathName()]; ok && current == mx {
		delete(s.muxers, mx.PathName())
	}
	s.muxersMutex.Unlock()
}

func (s *Server) getOrCreateMuxer(pathName string, author *session, sourceOnDemand bool) (*muxer, error) {
	s.muxersMutex.Lock()
	defer s.muxersMutex.Unlock()

	if s.closed.Load() {
		return nil, fmt.Errorf("terminated")
	}

	mux, ok := s.muxers[pathName]
	switch {
	case ok:
		return mux, nil
	case s.AlwaysRemux && !sourceOnDemand:
		return nil, fmt.Errorf("muxer is waiting to be created")
	default:
		return s.createMuxerLocked(pathName, author), nil
	}
}

func (s *Server) findSessionByUUIDLocked(id uuid.UUID) *session {
	for _, sx := range s.cdnSessionsByPath {
		if sx.uuid == id {
			return sx
		}
	}

	for _, sx := range s.sessionsBySecret {
		if sx.uuid == id {
			return sx
		}
	}

	return nil
}

// PathReady is called by pathManager.
func (s *Server) PathReady(pa defs.Path) {
	s.muxersMutex.Lock()
	defer s.muxersMutex.Unlock()

	if s.closed.Load() {
		return
	}

	s.createAutomaticMuxerLocked(pa)
}

// PathNotReady is called by pathManager.
func (s *Server) PathNotReady(pa defs.Path) {
	s.muxersMutex.Lock()

	if s.closed.Load() {
		s.muxersMutex.Unlock()
		return
	}

	mx, ok := s.muxers[pa.Name()]
	if ok && mx.author == nil {
		delete(s.muxers, pa.Name())
	} else {
		mx = nil
	}
	s.muxersMutex.Unlock()

	if mx != nil {
		mx.Close()
	}
}

// APIMuxersList implements defs.APIHLSServer.
func (s *Server) APIMuxersList() (*defs.APIHLSMuxerList, error) {
	if s.closed.Load() {
		return nil, fmt.Errorf("terminated")
	}

	s.muxersMutex.RLock()
	muxers := make([]*muxer, 0, len(s.muxers))
	for _, mx := range s.muxers {
		muxers = append(muxers, mx)
	}
	s.muxersMutex.RUnlock()

	data := &defs.APIHLSMuxerList{
		Items: make([]defs.APIHLSMuxer, 0, len(muxers)),
	}
	for _, mx := range muxers {
		data.Items = append(data.Items, *mx.apiItem())
	}

	sort.Slice(data.Items, func(i, j int) bool {
		return data.Items[i].Created.Before(data.Items[j].Created)
	})

	return data, nil
}

// APIMuxersGet implements defs.APIHLSServer.
func (s *Server) APIMuxersGet(name string) (*defs.APIHLSMuxer, error) {
	if s.closed.Load() {
		return nil, fmt.Errorf("terminated")
	}

	s.muxersMutex.RLock()
	mx, ok := s.muxers[name]
	s.muxersMutex.RUnlock()
	if !ok {
		return nil, ErrMuxerNotFound
	}

	return mx.apiItem(), nil
}

// APISessionsList implements defs.APIHLSServer.
func (s *Server) APISessionsList() (*defs.APIHLSSessionList, error) {
	if s.closed.Load() {
		return nil, fmt.Errorf("terminated")
	}

	s.sessionsMutex.RLock()
	sessions := make([]*session, 0, len(s.sessionsBySecret)+len(s.cdnSessionsByPath))
	for _, sx := range s.cdnSessionsByPath {
		sessions = append(sessions, sx)
	}
	for _, sx := range s.sessionsBySecret {
		sessions = append(sessions, sx)
	}
	s.sessionsMutex.RUnlock()

	data := &defs.APIHLSSessionList{
		Items: make([]defs.APIHLSSession, 0, len(sessions)),
	}
	for _, sx := range sessions {
		data.Items = append(data.Items, *sx.apiItem())
	}

	sort.Slice(data.Items, func(i, j int) bool {
		return data.Items[i].Created.Before(data.Items[j].Created)
	})

	return data, nil
}

// APISessionsGet implements defs.APIHLSServer.
func (s *Server) APISessionsGet(id uuid.UUID) (*defs.APIHLSSession, error) {
	if s.closed.Load() {
		return nil, fmt.Errorf("terminated")
	}

	s.sessionsMutex.RLock()
	sx := s.findSessionByUUIDLocked(id)
	s.sessionsMutex.RUnlock()
	if sx == nil {
		return nil, ErrSessionNotFound
	}

	return sx.apiItem(), nil
}

// APISessionsKick implements defs.APIHLSServer.
func (s *Server) APISessionsKick(id uuid.UUID) error {
	if s.closed.Load() {
		return fmt.Errorf("terminated")
	}

	s.sessionsMutex.Lock()
	sx := s.findSessionByUUIDLocked(id)
	if sx == nil {
		s.sessionsMutex.Unlock()
		return ErrSessionNotFound
	}

	if sx.isCDN {
		if current, ok := s.cdnSessionsByPath[sx.pathName]; ok && current == sx {
			delete(s.cdnSessionsByPath, sx.pathName)
		}
	} else if current, ok := s.sessionsBySecret[sx.secret]; ok && current == sx {
		delete(s.sessionsBySecret, sx.secret)
	}
	s.sessionsMutex.Unlock()

	sx.close2()
	return nil
}

func (s *Server) findOrCreateCDNSession(dir string) *session {
	s.sessionsMutex.Lock()
	defer s.sessionsMutex.Unlock()

	if s.closed.Load() {
		return nil
	}

	sx, ok := s.cdnSessionsByPath[dir]
	if ok {
		return sx
	}

	sx = &session{
		wg:              &s.wg,
		pathName:        dir,
		isCDN:           true,
		externalCmdPool: s.ExternalCmdPool,
		pathManager:     s.PathManager,
		server:          s,
	}
	sx.initialize()
	s.cdnSessionsByPath[dir] = sx
	return sx
}

func (s *Server) findNonCDNSession(dir string, ctx *gin.Context) (*session, error) {
	if s.closed.Load() {
		return nil, fmt.Errorf("server is closing")
	}

	secret := sessionGetSecret(ctx)
	if secret == nil {
		return nil, nil
	}

	s.sessionsMutex.RLock()
	sx, ok := s.sessionsBySecret[*secret]
	if !ok || sx.pathName != dir || sx.ip != ctx.ClientIP() {
		sx = nil
	}
	s.sessionsMutex.RUnlock()

	return sx, nil
}

func (s *Server) createNonCDNSession(dir string, ctx *gin.Context) *session {
	s.sessionsMutex.Lock()
	defer s.sessionsMutex.Unlock()

	if s.closed.Load() {
		return nil
	}

	sx := &session{
		wg:              &s.wg,
		remoteAddr:      httpp.RemoteAddr(ctx),
		pathName:        dir,
		query:           ctx.Request.URL.RawQuery,
		userAgent:       ctx.Request.UserAgent(),
		credentials:     httpp.Credentials(ctx.Request),
		externalCmdPool: s.ExternalCmdPool,
		pathManager:     s.PathManager,
		server:          s,
	}
	sx.initialize()
	s.sessionsBySecret[sx.secret] = sx
	return sx
}

// closeSession is called by session.
func (s *Server) closeSession(sx *session) {
	s.sessionsMutex.Lock()
	if sx.isCDN {
		if current, ok := s.cdnSessionsByPath[sx.pathName]; ok && current == sx {
			delete(s.cdnSessionsByPath, sx.pathName)
		}
	} else if current, ok := s.sessionsBySecret[sx.secret]; ok && current == sx {
		delete(s.sessionsBySecret, sx.secret)
	}
	s.sessionsMutex.Unlock()
}
