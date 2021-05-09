package rtspserver

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"strconv"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"

	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/pathman"
	"github.com/aler9/rtsp-simple-server/internal/rtspconn"
	"github.com/aler9/rtsp-simple-server/internal/rtspsession"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

func newSessionVisualID(sessions map[*gortsplib.ServerSession]*rtspsession.Session) (string, error) {
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

// Parent is implemented by program.
type Parent interface {
	Log(logger.Level, string, ...interface{})
}

// Server is a RTSP server.
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
	conns    map[*gortsplib.ServerConn]*rtspconn.Conn
	sessions map[*gortsplib.ServerSession]*rtspsession.Session

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
		conns:       make(map[*gortsplib.ServerConn]*rtspconn.Conn),
		sessions:    make(map[*gortsplib.ServerSession]*rtspsession.Session),
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

// OnConnOpen implements gortsplib.ServerHandlerOnConnOpen.
func (s *Server) OnConnOpen(ctx *gortsplib.ServerHandlerOnConnOpenCtx) {
	c := rtspconn.New(
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
func (s *Server) OnConnClose(ctx *gortsplib.ServerHandlerOnConnCloseCtx) {
	s.mutex.Lock()
	c := s.conns[ctx.Conn]
	delete(s.conns, ctx.Conn)
	s.mutex.Unlock()

	c.ParentClose(ctx.Error)
}

// OnRequest implements gortsplib.ServerHandlerOnRequest.
func (s *Server) OnRequest(sc *gortsplib.ServerConn, req *base.Request) {
	s.mutex.Lock()
	c := s.conns[sc]
	s.mutex.Unlock()

	c.OnRequest(req)
}

// OnResponse implements gortsplib.ServerHandlerOnResponse.
func (s *Server) OnResponse(sc *gortsplib.ServerConn, res *base.Response) {
	s.mutex.Lock()
	c := s.conns[sc]
	s.mutex.Unlock()

	c.OnResponse(res)
}

// OnSessionOpen implements gortsplib.ServerHandlerOnSessionOpen.
func (s *Server) OnSessionOpen(ctx *gortsplib.ServerHandlerOnSessionOpenCtx) {
	s.mutex.Lock()

	// do not use ss.ID() in logs, since it allows to take ownership of a session
	// use a new random ID
	visualID, _ := newSessionVisualID(s.sessions)

	se := rtspsession.New(
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
func (s *Server) OnSessionClose(ctx *gortsplib.ServerHandlerOnSessionCloseCtx) {
	s.mutex.Lock()
	se := s.sessions[ctx.Session]
	delete(s.sessions, ctx.Session)
	s.mutex.Unlock()

	se.ParentClose()
}

// OnDescribe implements gortsplib.ServerHandlerOnDescribe.
func (s *Server) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, []byte, error) {
	s.mutex.RLock()
	c := s.conns[ctx.Conn]
	s.mutex.RUnlock()
	return c.OnDescribe(ctx)
}

// OnAnnounce implements gortsplib.ServerHandlerOnAnnounce.
func (s *Server) OnAnnounce(ctx *gortsplib.ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	s.mutex.RLock()
	c := s.conns[ctx.Conn]
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	return se.OnAnnounce(c, ctx)
}

// OnSetup implements gortsplib.ServerHandlerOnSetup.
func (s *Server) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, error) {
	s.mutex.RLock()
	c := s.conns[ctx.Conn]
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	return se.OnSetup(c, ctx)
}

// OnPlay implements gortsplib.ServerHandlerOnPlay.
func (s *Server) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	s.mutex.RLock()
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	return se.OnPlay(ctx)
}

// OnRecord implements gortsplib.ServerHandlerOnRecord.
func (s *Server) OnRecord(ctx *gortsplib.ServerHandlerOnRecordCtx) (*base.Response, error) {
	s.mutex.RLock()
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	return se.OnRecord(ctx)
}

// OnPause implements gortsplib.ServerHandlerOnPause.
func (s *Server) OnPause(ctx *gortsplib.ServerHandlerOnPauseCtx) (*base.Response, error) {
	s.mutex.RLock()
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	return se.OnPause(ctx)
}

// OnFrame implements gortsplib.ServerHandlerOnFrame.
func (s *Server) OnFrame(ctx *gortsplib.ServerHandlerOnFrameCtx) {
	s.mutex.RLock()
	se := s.sessions[ctx.Session]
	s.mutex.RUnlock()
	se.OnIncomingFrame(ctx)
}
