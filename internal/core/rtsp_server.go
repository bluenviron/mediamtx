package core

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/headers"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type rtspServerParent interface {
	Log(logger.Level, string, ...interface{})
}

type rtspServer struct {
	authMethods         []headers.AuthMethod
	readTimeout         conf.StringDuration
	isTLS               bool
	rtspAddress         string
	protocols           map[conf.Protocol]struct{}
	runOnConnect        string
	runOnConnectRestart bool
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
	parentCtx context.Context,
	address string,
	authMethods []headers.AuthMethod,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	readBufferCount int,
	readBufferSize int,
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
	metrics *metrics,
	pathManager *pathManager,
	parent rtspServerParent) (*rtspServer, error) {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	s := &rtspServer{
		authMethods: authMethods,
		readTimeout: readTimeout,
		isTLS:       isTLS,
		rtspAddress: rtspAddress,
		protocols:   protocols,
		metrics:     metrics,
		pathManager: pathManager,
		parent:      parent,
		ctx:         ctx,
		ctxCancel:   ctxCancel,
		conns:       make(map[*gortsplib.ServerConn]*rtspConn),
		sessions:    make(map[*gortsplib.ServerSession]*rtspSession),
	}

	s.srv = &gortsplib.Server{
		Handler:         s,
		ReadTimeout:     time.Duration(readTimeout),
		WriteTimeout:    time.Duration(writeTimeout),
		ReadBufferCount: readBufferCount,
		ReadBufferSize:  readBufferSize,
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

	err := s.srv.Start(address)
	if err != nil {
		return nil, err
	}

	if s.srv.UDPRTPAddress != "" {
		s.Log(logger.Info, "UDP/RTP listener opened on %s", s.srv.UDPRTPAddress)
	}

	if s.srv.UDPRTCPAddress != "" {
		s.Log(logger.Info, "UDP/RTCP listener opened on %s", s.srv.UDPRTCPAddress)
	}

	s.Log(logger.Info, "TCP listener opened on %s", address)

	if s.metrics != nil {
		if !isTLS {
			s.metrics.OnRTSPServerSet(s)
		} else {
			s.metrics.OnRTSPSServerSet(s)
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
	s.ctxCancel()
	s.wg.Wait()
	s.Log(logger.Info, "closed")
}

func (s *rtspServer) run() {
	defer s.wg.Done()

	s.wg.Add(1)
	serverErr := make(chan error)
	go func() {
		defer s.wg.Done()

		err := s.srv.Wait()

		select {
		case serverErr <- err:
		case <-s.ctx.Done():
		}
	}()

outer:
	select {
	case err := <-serverErr:
		s.Log(logger.Warn, "ERR: %s", err)
		break outer

	case <-s.ctx.Done():
		break outer
	}

	s.ctxCancel()

	s.srv.Close()

	if s.metrics != nil {
		if !s.isTLS {
			s.metrics.OnRTSPServerSet(nil)
		} else {
			s.metrics.OnRTSPSServerSet(nil)
		}
	}
}

func (s *rtspServer) newSessionID() (string, error) {
	for {
		b := make([]byte, 4)
		_, err := rand.Read(b)
		if err != nil {
			return "", err
		}

		u := binary.LittleEndian.Uint32(b)
		u %= 899999999
		u += 100000000

		id := strconv.FormatUint(uint64(u), 10)

		alreadyPresent := func() bool {
			for _, s := range s.sessions {
				if s.ID() == id {
					return true
				}
			}
			return false
		}()
		if !alreadyPresent {
			return id, nil
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
		s.pathManager,
		ctx.Conn,
		s)

	s.mutex.Lock()
	s.conns[ctx.Conn] = c
	s.mutex.Unlock()
}

// OnConnClose implements gortsplib.ServerHandlerOnConnClose.
func (s *rtspServer) OnConnClose(ctx *gortsplib.ServerHandlerOnConnCloseCtx) {
	s.mutex.Lock()
	c := s.conns[ctx.Conn]
	delete(s.conns, ctx.Conn)
	s.mutex.Unlock()

	c.OnClose(ctx.Error)
}

// OnRequest implements gortsplib.ServerHandlerOnRequest.
func (s *rtspServer) OnRequest(sc *gortsplib.ServerConn, req *base.Request) {
	s.mutex.Lock()
	c := s.conns[sc]
	s.mutex.Unlock()

	c.OnRequest(req)
}

// OnResponse implements gortsplib.ServerHandlerOnResponse.
func (s *rtspServer) OnResponse(sc *gortsplib.ServerConn, res *base.Response) {
	s.mutex.Lock()
	c := s.conns[sc]
	s.mutex.Unlock()

	c.OnResponse(res)
}

// OnSessionOpen implements gortsplib.ServerHandlerOnSessionOpen.
func (s *rtspServer) OnSessionOpen(ctx *gortsplib.ServerHandlerOnSessionOpenCtx) {
	s.mutex.Lock()

	id, _ := s.newSessionID()

	se := newRTSPSession(
		s.isTLS,
		s.rtspAddress,
		s.protocols,
		id,
		ctx.Session,
		ctx.Conn,
		s.pathManager,
		s)

	s.sessions[ctx.Session] = se
	s.mutex.Unlock()
}

// OnSessionClose implements gortsplib.ServerHandlerOnSessionClose.
func (s *rtspServer) OnSessionClose(ctx *gortsplib.ServerHandlerOnSessionCloseCtx) {
	s.mutex.Lock()
	se := s.sessions[ctx.Session]
	delete(s.sessions, ctx.Session)
	s.mutex.Unlock()

	if se != nil {
		se.OnClose()
	}
}

// OnDescribe implements gortsplib.ServerHandlerOnDescribe.
func (s *rtspServer) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	s.mutex.RLock()
	c := s.conns[ctx.Conn]
	s.mutex.RUnlock()
	return c.OnDescribe(ctx)
}

// OnAnnounce implements gortsplib.ServerHandlerOnAnnounce.
func (s *rtspServer) OnAnnounce(ctx *gortsplib.ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	s.mutex.RLock()
	c := s.conns[ctx.Conn]
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	return se.OnAnnounce(c, ctx)
}

// OnSetup implements gortsplib.ServerHandlerOnSetup.
func (s *rtspServer) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	s.mutex.RLock()
	c := s.conns[ctx.Conn]
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	return se.OnSetup(c, ctx)
}

// OnPlay implements gortsplib.ServerHandlerOnPlay.
func (s *rtspServer) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	s.mutex.RLock()
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	return se.OnPlay(ctx)
}

// OnRecord implements gortsplib.ServerHandlerOnRecord.
func (s *rtspServer) OnRecord(ctx *gortsplib.ServerHandlerOnRecordCtx) (*base.Response, error) {
	s.mutex.RLock()
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	return se.OnRecord(ctx)
}

// OnPause implements gortsplib.ServerHandlerOnPause.
func (s *rtspServer) OnPause(ctx *gortsplib.ServerHandlerOnPauseCtx) (*base.Response, error) {
	s.mutex.RLock()
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	return se.OnPause(ctx)
}

// OnFrame implements gortsplib.ServerHandlerOnFrame.
func (s *rtspServer) OnFrame(ctx *gortsplib.ServerHandlerOnFrameCtx) {
	s.mutex.RLock()
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	se.OnFrame(ctx)
}

// OnAPIRTSPSessionsList is called by api and metrics.
func (s *rtspServer) OnAPIRTSPSessionsList(req apiRTSPSessionsListReq) apiRTSPSessionsListRes {
	select {
	case <-s.ctx.Done():
		return apiRTSPSessionsListRes{Err: fmt.Errorf("terminated")}
	default:
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	data := &apiRTSPSessionsListData{
		Items: make(map[string]apiRTSPSessionsListItem),
	}

	for _, s := range s.sessions {
		data.Items[s.ID()] = apiRTSPSessionsListItem{
			RemoteAddr: s.RemoteAddr().String(),
			State: func() string {
				switch s.safeState() {
				case gortsplib.ServerSessionStatePreRead,
					gortsplib.ServerSessionStateRead:
					return "read"

				case gortsplib.ServerSessionStatePrePublish,
					gortsplib.ServerSessionStatePublish:
					return "publish"
				}
				return "idle"
			}(),
		}
	}

	return apiRTSPSessionsListRes{Data: data}
}

// OnAPIRTSPSessionsKick is called by api.
func (s *rtspServer) OnAPIRTSPSessionsKick(req apiRTSPSessionsKickReq) apiRTSPSessionsKickRes {
	select {
	case <-s.ctx.Done():
		return apiRTSPSessionsKickRes{Err: fmt.Errorf("terminated")}
	default:
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for key, se := range s.sessions {
		if se.ID() == req.ID {
			se.Close()
			delete(s.sessions, key)
			se.OnClose()
			return apiRTSPSessionsKickRes{}
		}
	}

	return apiRTSPSessionsKickRes{Err: fmt.Errorf("not found")}
}
