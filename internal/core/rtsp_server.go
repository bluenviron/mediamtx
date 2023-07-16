package core

import (
	"context"
	"crypto/tls"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v3"
	"github.com/bluenviron/gortsplib/v3/pkg/base"
	"github.com/bluenviron/gortsplib/v3/pkg/headers"
	"github.com/bluenviron/gortsplib/v3/pkg/liberrors"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
)

type rtspServerParent interface {
	logger.Writer
}

func printAddresses(srv *gortsplib.Server) string {
	var ret []string

	ret = append(ret, fmt.Sprintf("%s (TCP)", srv.RTSPAddress))

	if srv.UDPRTPAddress != "" {
		ret = append(ret, fmt.Sprintf("%s (UDP/RTP)", srv.UDPRTPAddress))
	}

	if srv.UDPRTCPAddress != "" {
		ret = append(ret, fmt.Sprintf("%s (UDP/RTCP)", srv.UDPRTCPAddress))
	}

	return strings.Join(ret, ", ")
}

type rtspServer struct {
	authMethods         []headers.AuthMethod
	readTimeout         conf.StringDuration
	isTLS               bool
	rtspAddress         string
	protocols           map[conf.Protocol]struct{}
	runOnConnect        string
	runOnConnectRestart bool
	externalCmdPool     *externalcmd.Pool
	metrics             *metrics
	pathManager         *pathManager
	parent              rtspServerParent

	ctx       context.Context
	ctxCancel func()
	wg        sync.WaitGroup
	srv       *gortsplib.Server
	mutex     sync.RWMutex
	conns     map[*gortsplib.ServerConn]*rtspConn
	sessions  map[*gortsplib.ServerSession]*rtspSession
}

func newRTSPServer(
	address string,
	authMethods []headers.AuthMethod,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	readBufferCount int,
	useUDP bool,
	useMulticast bool,
	rtpAddress string,
	rtcpAddress string,
	multicastIPRange string,
	multicastRTPPort int,
	multicastRTCPPort int,
	isTLS bool,
	serverCert string,
	serverKey string,
	rtspAddress string,
	protocols map[conf.Protocol]struct{},
	runOnConnect string,
	runOnConnectRestart bool,
	externalCmdPool *externalcmd.Pool,
	metrics *metrics,
	pathManager *pathManager,
	parent rtspServerParent,
) (*rtspServer, error) {
	ctx, ctxCancel := context.WithCancel(context.Background())

	s := &rtspServer{
		authMethods:         authMethods,
		readTimeout:         readTimeout,
		isTLS:               isTLS,
		rtspAddress:         rtspAddress,
		protocols:           protocols,
		runOnConnect:        runOnConnect,
		runOnConnectRestart: runOnConnectRestart,
		externalCmdPool:     externalCmdPool,
		metrics:             metrics,
		pathManager:         pathManager,
		parent:              parent,
		ctx:                 ctx,
		ctxCancel:           ctxCancel,
		conns:               make(map[*gortsplib.ServerConn]*rtspConn),
		sessions:            make(map[*gortsplib.ServerSession]*rtspSession),
	}

	s.srv = &gortsplib.Server{
		Handler:          s,
		ReadTimeout:      time.Duration(readTimeout),
		WriteTimeout:     time.Duration(writeTimeout),
		ReadBufferCount:  readBufferCount,
		WriteBufferCount: readBufferCount,
		RTSPAddress:      address,
	}

	if useUDP {
		s.srv.UDPRTPAddress = rtpAddress
		s.srv.UDPRTCPAddress = rtcpAddress
	}

	if useMulticast {
		s.srv.MulticastIPRange = multicastIPRange
		s.srv.MulticastRTPPort = multicastRTPPort
		s.srv.MulticastRTCPPort = multicastRTCPPort
	}

	if isTLS {
		cert, err := tls.LoadX509KeyPair(serverCert, serverKey)
		if err != nil {
			return nil, err
		}

		s.srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	}

	err := s.srv.Start()
	if err != nil {
		return nil, err
	}

	s.Log(logger.Info, "listener opened on %s", printAddresses(s.srv))

	if metrics != nil {
		if !isTLS {
			metrics.rtspServerSet(s)
		} else {
			metrics.rtspsServerSet(s)
		}
	}

	s.wg.Add(1)
	go s.run()

	return s, nil
}

func (s *rtspServer) Log(level logger.Level, format string, args ...interface{}) {
	label := func() string {
		if s.isTLS {
			return "RTSPS"
		}
		return "RTSP"
	}()
	s.parent.Log(level, "[%s] "+format, append([]interface{}{label}, args...)...)
}

func (s *rtspServer) close() {
	s.Log(logger.Info, "listener is closing")
	s.ctxCancel()
	s.wg.Wait()
}

func (s *rtspServer) run() {
	defer s.wg.Done()

	serverErr := make(chan error)
	go func() {
		serverErr <- s.srv.Wait()
	}()

outer:
	select {
	case err := <-serverErr:
		s.Log(logger.Error, "%s", err)
		break outer

	case <-s.ctx.Done():
		s.srv.Close()
		<-serverErr
		break outer
	}

	s.ctxCancel()

	if s.metrics != nil {
		if !s.isTLS {
			s.metrics.rtspServerSet(nil)
		} else {
			s.metrics.rtspsServerSet(nil)
		}
	}
}

// OnConnOpen implements gortsplib.ServerHandlerOnConnOpen.
func (s *rtspServer) OnConnOpen(ctx *gortsplib.ServerHandlerOnConnOpenCtx) {
	c := newRTSPConn(
		s.rtspAddress,
		s.authMethods,
		s.readTimeout,
		s.runOnConnect,
		s.runOnConnectRestart,
		s.externalCmdPool,
		s.pathManager,
		ctx.Conn,
		s)
	s.mutex.Lock()
	s.conns[ctx.Conn] = c
	s.mutex.Unlock()

	ctx.Conn.SetUserData(c)
}

// OnConnClose implements gortsplib.ServerHandlerOnConnClose.
func (s *rtspServer) OnConnClose(ctx *gortsplib.ServerHandlerOnConnCloseCtx) {
	s.mutex.Lock()
	c := s.conns[ctx.Conn]
	delete(s.conns, ctx.Conn)
	s.mutex.Unlock()
	c.onClose(ctx.Error)
}

// OnRequest implements gortsplib.ServerHandlerOnRequest.
func (s *rtspServer) OnRequest(sc *gortsplib.ServerConn, req *base.Request) {
	c := sc.UserData().(*rtspConn)
	c.onRequest(req)
}

// OnResponse implements gortsplib.ServerHandlerOnResponse.
func (s *rtspServer) OnResponse(sc *gortsplib.ServerConn, res *base.Response) {
	c := sc.UserData().(*rtspConn)
	c.OnResponse(res)
}

// OnSessionOpen implements gortsplib.ServerHandlerOnSessionOpen.
func (s *rtspServer) OnSessionOpen(ctx *gortsplib.ServerHandlerOnSessionOpenCtx) {
	se := newRTSPSession(
		s.isTLS,
		s.protocols,
		ctx.Session,
		ctx.Conn,
		s.externalCmdPool,
		s.pathManager,
		s)
	s.mutex.Lock()
	s.sessions[ctx.Session] = se
	s.mutex.Unlock()
	ctx.Session.SetUserData(se)
}

// OnSessionClose implements gortsplib.ServerHandlerOnSessionClose.
func (s *rtspServer) OnSessionClose(ctx *gortsplib.ServerHandlerOnSessionCloseCtx) {
	s.mutex.Lock()
	se := s.sessions[ctx.Session]
	delete(s.sessions, ctx.Session)
	s.mutex.Unlock()

	if se != nil {
		se.onClose(ctx.Error)
	}
}

// OnDescribe implements gortsplib.ServerHandlerOnDescribe.
func (s *rtspServer) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	c := ctx.Conn.UserData().(*rtspConn)
	return c.onDescribe(ctx)
}

// OnAnnounce implements gortsplib.ServerHandlerOnAnnounce.
func (s *rtspServer) OnAnnounce(ctx *gortsplib.ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	c := ctx.Conn.UserData().(*rtspConn)
	se := ctx.Session.UserData().(*rtspSession)
	return se.onAnnounce(c, ctx)
}

// OnSetup implements gortsplib.ServerHandlerOnSetup.
func (s *rtspServer) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	c := ctx.Conn.UserData().(*rtspConn)
	se := ctx.Session.UserData().(*rtspSession)
	return se.onSetup(c, ctx)
}

// OnPlay implements gortsplib.ServerHandlerOnPlay.
func (s *rtspServer) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	se := ctx.Session.UserData().(*rtspSession)
	return se.onPlay(ctx)
}

// OnRecord implements gortsplib.ServerHandlerOnRecord.
func (s *rtspServer) OnRecord(ctx *gortsplib.ServerHandlerOnRecordCtx) (*base.Response, error) {
	se := ctx.Session.UserData().(*rtspSession)
	return se.onRecord(ctx)
}

// OnPause implements gortsplib.ServerHandlerOnPause.
func (s *rtspServer) OnPause(ctx *gortsplib.ServerHandlerOnPauseCtx) (*base.Response, error) {
	se := ctx.Session.UserData().(*rtspSession)
	return se.onPause(ctx)
}

// OnPacketLost implements gortsplib.ServerHandlerOnDecodeError.
func (s *rtspServer) OnPacketLost(ctx *gortsplib.ServerHandlerOnPacketLostCtx) {
	se := ctx.Session.UserData().(*rtspSession)
	se.onPacketLost(ctx)
}

// OnDecodeError implements gortsplib.ServerHandlerOnDecodeError.
func (s *rtspServer) OnDecodeError(ctx *gortsplib.ServerHandlerOnDecodeErrorCtx) {
	se := ctx.Session.UserData().(*rtspSession)
	se.onDecodeError(ctx)
}

func (s *rtspServer) findConnByUUID(uuid uuid.UUID) *rtspConn {
	for _, c := range s.conns {
		if c.uuid == uuid {
			return c
		}
	}
	return nil
}

func (s *rtspServer) findSessionByUUID(uuid uuid.UUID) (*gortsplib.ServerSession, *rtspSession) {
	for key, sx := range s.sessions {
		if sx.uuid == uuid {
			return key, sx
		}
	}
	return nil, nil
}

// apiConnsList is called by api and metrics.
func (s *rtspServer) apiConnsList() (*apiRTSPConnsList, error) {
	select {
	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	default:
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	data := &apiRTSPConnsList{
		Items: []*apiRTSPConn{},
	}

	for _, c := range s.conns {
		data.Items = append(data.Items, c.apiItem())
	}

	sort.Slice(data.Items, func(i, j int) bool {
		return data.Items[i].Created.Before(data.Items[j].Created)
	})

	return data, nil
}

// apiConnsGet is called by api.
func (s *rtspServer) apiConnsGet(uuid uuid.UUID) (*apiRTSPConn, error) {
	select {
	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	default:
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	conn := s.findConnByUUID(uuid)
	if conn == nil {
		return nil, errAPINotFound
	}

	return conn.apiItem(), nil
}

// apiSessionsList is called by api and metrics.
func (s *rtspServer) apiSessionsList() (*apiRTSPSessionsList, error) {
	select {
	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	default:
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	data := &apiRTSPSessionsList{
		Items: []*apiRTSPSession{},
	}

	for _, s := range s.sessions {
		data.Items = append(data.Items, s.apiItem())
	}

	sort.Slice(data.Items, func(i, j int) bool {
		return data.Items[i].Created.Before(data.Items[j].Created)
	})

	return data, nil
}

// apiSessionsGet is called by api.
func (s *rtspServer) apiSessionsGet(uuid uuid.UUID) (*apiRTSPSession, error) {
	select {
	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	default:
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	_, sx := s.findSessionByUUID(uuid)
	if sx == nil {
		return nil, errAPINotFound
	}

	return sx.apiItem(), nil
}

// apiSessionsKick is called by api.
func (s *rtspServer) apiSessionsKick(uuid uuid.UUID) error {
	select {
	case <-s.ctx.Done():
		return fmt.Errorf("terminated")
	default:
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	key, sx := s.findSessionByUUID(uuid)
	if sx == nil {
		return errAPINotFound
	}

	sx.close()
	delete(s.sessions, key)
	sx.onClose(liberrors.ErrServerTerminated{})
	return nil
}
