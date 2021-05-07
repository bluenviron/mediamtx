package serverrtsp

import (
	"crypto/tls"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"

	"github.com/aler9/rtsp-simple-server/internal/clientrtsp"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/pathman"
	"github.com/aler9/rtsp-simple-server/internal/sessionrtsp"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

// Parent is implemented by program.
type Parent interface {
	Log(logger.Level, string, ...interface{})
}

// Server is a RTSP listener.
type Server struct {
	readTimeout         time.Duration
	isTLS               bool
	rtspAddress         string
	protocols           map[base.StreamProtocol]struct{}
	runOnConnect        string
	runOnConnectRestart bool
	stats               *stats.Stats
	pathMan             *pathman.PathManager
	parent              Parent

	srv      *gortsplib.Server
	mutex    sync.RWMutex
	clients  map[*gortsplib.ServerConn]*clientrtsp.Client
	sessions map[*gortsplib.ServerSession]*sessionrtsp.Session

	// in
	terminate chan struct{}

	// out
	done chan struct{}
}

// New allocates a Server.
func New(
	address string,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	readBufferCount int,
	readBufferSize int,
	useUDP bool,
	rtpAddress string,
	rtcpAddress string,
	isTLS bool,
	serverCert string,
	serverKey string,
	rtspAddress string,
	protocols map[base.StreamProtocol]struct{},
	runOnConnect string,
	runOnConnectRestart bool,
	stats *stats.Stats,
	pathMan *pathman.PathManager,
	parent Parent) (*Server, error) {

	s := &Server{
		readTimeout: readTimeout,
		isTLS:       isTLS,
		rtspAddress: rtspAddress,
		protocols:   protocols,
		stats:       stats,
		pathMan:     pathMan,
		parent:      parent,
		clients:     make(map[*gortsplib.ServerConn]*clientrtsp.Client),
		sessions:    make(map[*gortsplib.ServerSession]*sessionrtsp.Session),
		terminate:   make(chan struct{}),
		done:        make(chan struct{}),
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

	go s.run()

	return s, nil
}

// Log is the main logging function.
func (s *Server) Log(level logger.Level, format string, args ...interface{}) {
	label := func() string {
		if s.isTLS {
			return "RTSPS"
		}
		return "RTSP"
	}()
	s.parent.Log(level, "[%s] "+format, append([]interface{}{label}, args...)...)
}

// Close closes a Server.
func (s *Server) Close() {
	close(s.terminate)
	<-s.done
}

func (s *Server) run() {
	defer close(s.done)

	serverDone := make(chan struct{})
	serverErr := make(chan error)
	go func() {
		defer close(serverDone)
		serverErr <- s.srv.Wait()
	}()

outer:
	select {
	case err := <-serverErr:
		s.Log(logger.Warn, "ERR: %s", err)
		break outer

	case <-s.terminate:
		break outer
	}

	go func() {
		for range serverErr {
		}
	}()

	s.srv.Close()

	<-serverDone

	close(serverErr)
}

// OnConnOpen implements gortsplib.ServerHandlerOnConnOpenCtx.
func (s *Server) OnConnOpen(sc *gortsplib.ServerConn) {
	c := clientrtsp.New(
		s.rtspAddress,
		s.readTimeout,
		s.runOnConnect,
		s.runOnConnectRestart,
		s.pathMan,
		s.stats,
		sc,
		s)

	s.mutex.Lock()
	s.clients[sc] = c
	s.mutex.Unlock()
}

// OnConnClose implements gortsplib.ServerHandlerOnConnCloseCtx.
func (s *Server) OnConnClose(sc *gortsplib.ServerConn, err error) {
	s.mutex.Lock()
	c := s.clients[sc]
	delete(s.clients, sc)
	s.mutex.Unlock()

	c.Close(err)
}

// OnRequest implements gortsplib.ServerHandlerOnRequestCtx.
func (s *Server) OnRequest(sc *gortsplib.ServerConn, req *base.Request) {
	s.mutex.Lock()
	c := s.clients[sc]
	s.mutex.Unlock()

	c.OnRequest(req)
}

// OnResponse implements gortsplib.ServerHandlerOnResponseCtx.
func (s *Server) OnResponse(sc *gortsplib.ServerConn, res *base.Response) {
	s.mutex.Lock()
	c := s.clients[sc]
	s.mutex.Unlock()

	c.OnResponse(res)
}

// OnSessionOpen implements gortsplib.ServerHandlerOnSessionOpenCtx.
func (s *Server) OnSessionOpen(ss *gortsplib.ServerSession) {
	se := sessionrtsp.New(
		s.rtspAddress,
		s.protocols,
		ss,
		s.pathMan,
		s)

	s.mutex.Lock()
	s.sessions[ss] = se
	s.mutex.Unlock()
}

// OnSessionClose implements gortsplib.ServerHandlerOnSessionCloseCtx.
func (s *Server) OnSessionClose(ss *gortsplib.ServerSession, err error) {
	s.mutex.Lock()
	se := s.sessions[ss]
	delete(s.sessions, ss)
	s.mutex.Unlock()

	se.Close()
}

// OnDescribe implements gortsplib.ServerHandlerOnDescribeCtx.
func (s *Server) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, []byte, error) {
	s.mutex.RLock()
	c := s.clients[ctx.Conn]
	s.mutex.RUnlock()
	return c.OnDescribe(ctx)
}

// OnAnnounce implements gortsplib.ServerHandlerOnAnnounceCtx.
func (s *Server) OnAnnounce(ctx *gortsplib.ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	s.mutex.RLock()
	c := s.clients[ctx.Conn]
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	return se.OnAnnounce(c, ctx)
}

// OnSetup implements gortsplib.ServerHandlerOnSetupCtx.
func (s *Server) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, error) {
	s.mutex.RLock()
	c := s.clients[ctx.Conn]
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	return se.OnSetup(c, ctx)
}

// OnPlay implements gortsplib.ServerHandlerOnPlayCtx.
func (s *Server) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	s.mutex.RLock()
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	return se.OnPlay(ctx)
}

// OnRecord implements gortsplib.ServerHandlerOnRecordCtx.
func (s *Server) OnRecord(ctx *gortsplib.ServerHandlerOnRecordCtx) (*base.Response, error) {
	s.mutex.RLock()
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	return se.OnRecord(ctx)
}

// OnPause implements gortsplib.ServerHandlerOnPauseCtx.
func (s *Server) OnPause(ctx *gortsplib.ServerHandlerOnPauseCtx) (*base.Response, error) {
	s.mutex.RLock()
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	return se.OnPause(ctx)
}

// OnFrame implements gortsplib.ServerHandlerOnFrameCtx.
func (s *Server) OnFrame(ctx *gortsplib.ServerHandlerOnFrameCtx) {
	s.mutex.RLock()
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	se.OnIncomingFrame(ctx)
}
