package core

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"strconv"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

func newSessionVisualID(sessions map[*gortsplib.ServerSession]*rtspSession) (string, error) {
	for {
		b := make([]byte, 4)
		_, err := rand.Read(b)
		if err != nil {
			return "", err
		}

		id := strconv.FormatUint(uint64(binary.LittleEndian.Uint32(b)), 10)

		alreadyPresent := func() bool {
			for _, s := range sessions {
				if s.VisualID() == id {
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

type rtspServerParent interface {
	Log(logger.Level, string, ...interface{})
}

type rtspServer struct {
	readTimeout         time.Duration
	isTLS               bool
	rtspAddress         string
	protocols           map[conf.Protocol]struct{}
	runOnConnect        string
	runOnConnectRestart bool
	stats               *stats
	pathMan             *pathManager
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
	readTimeout time.Duration,
	writeTimeout time.Duration,
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
	stats *stats,
	pathMan *pathManager,
	parent rtspServerParent) (*rtspServer, error) {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	s := &rtspServer{
		readTimeout: readTimeout,
		isTLS:       isTLS,
		rtspAddress: rtspAddress,
		protocols:   protocols,
		stats:       stats,
		pathMan:     pathMan,
		parent:      parent,
		ctx:         ctx,
		ctxCancel:   ctxCancel,
		conns:       make(map[*gortsplib.ServerConn]*rtspConn),
		sessions:    make(map[*gortsplib.ServerSession]*rtspSession),
	}

	s.srv = &gortsplib.Server{
		Handler:         s,
		ReadTimeout:     readTimeout,
		WriteTimeout:    writeTimeout,
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
}

// OnConnOpen implements gortsplib.ServerHandlerOnConnOpen.
func (s *rtspServer) OnConnOpen(ctx *gortsplib.ServerHandlerOnConnOpenCtx) {
	c := newRTSPConn(
		s.rtspAddress,
		s.readTimeout,
		s.runOnConnect,
		s.runOnConnectRestart,
		s.pathMan,
		s.stats,
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

	c.ParentClose(ctx.Error)
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

	// do not use ss.ID() in logs, since it allows to take ownership of a session
	// use a new random ID
	visualID, _ := newSessionVisualID(s.sessions)

	se := newRTSPSession(
		s.rtspAddress,
		s.protocols,
		visualID,
		ctx.Session,
		ctx.Conn,
		s.pathMan,
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

	se.ParentClose()
}

// OnDescribe implements gortsplib.ServerHandlerOnDescribe.
func (s *rtspServer) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
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
	se.OnIncomingFrame(ctx)
}
