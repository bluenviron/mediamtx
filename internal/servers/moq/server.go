// Package moq contains a Media over QUIC server.
package moq

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/google/uuid"
	"github.com/quic-go/webtransport-go"
)

// ErrSessionNotFound is returned when a session is not found.
var ErrSessionNotFound = errors.New("session not found")

func interfaceIsEmpty(i any) bool {
	return reflect.ValueOf(i).Kind() != reflect.Pointer || reflect.ValueOf(i).IsNil()
}

type serverPathManager interface {
	FindPathConf(req defs.PathFindPathConfReq) (*defs.PathFindPathConfRes, error)
	AddReader(req defs.PathAddReaderReq) (*defs.PathAddReaderRes, error)
	AddPublisher(req defs.PathAddPublisherReq) (*defs.PathAddPublisherRes, error)
}

type newSessionRes struct {
	sx  *session
	err error
}

type newSessionReq struct {
	pathName  string
	query     string
	userAgent string
	wt        *webtransport.Session
	res       chan newSessionRes
}

type serverAPISessionsListRes struct {
	data *defs.APIMoQSessionList
}

type serverAPISessionsListReq struct {
	res chan serverAPISessionsListRes
}

type serverAPISessionsGetRes struct {
	data *defs.APIMoQSession
	err  error
}

type serverAPISessionsGetReq struct {
	uuid uuid.UUID
	res  chan serverAPISessionsGetRes
}

type serverAPISessionsKickRes struct {
	err error
}

type serverAPISessionsKickReq struct {
	uuid uuid.UUID
	res  chan serverAPISessionsKickRes
}

type serverParent interface {
	logger.Writer
}

type serverMetrics interface {
	SetMoQServer(defs.APIMoQServer)
}

// Server is a Media over QUIC server.
type Server struct {
	HTTP2Address      string
	HTTP3Address      string
	ServerKey         string
	ServerCert        string
	AllowOrigins      []string
	UDPReadBufferSize uint
	TrustedProxies    conf.IPNetworks
	ReadTimeout       conf.Duration
	WriteTimeout      conf.Duration
	PathManager       serverPathManager
	Metrics           serverMetrics
	Parent            serverParent

	ctx        context.Context
	ctxCancel  context.CancelFunc
	httpServer *httpServer
	sessions   map[*session]struct{}

	chNewSession      chan newSessionReq
	chCloseSession    chan *session
	chAPISessionsList chan serverAPISessionsListReq
	chAPISessionsGet  chan serverAPISessionsGetReq
	chAPISessionsKick chan serverAPISessionsKickReq
	done              chan struct{}
}

// Initialize initializes the server.
func (s *Server) Initialize() error {
	ctx, ctxCancel := context.WithCancel(context.Background())

	s.ctx = ctx
	s.ctxCancel = ctxCancel
	s.sessions = make(map[*session]struct{})
	s.chNewSession = make(chan newSessionReq)
	s.chCloseSession = make(chan *session)
	s.chAPISessionsList = make(chan serverAPISessionsListReq)
	s.chAPISessionsGet = make(chan serverAPISessionsGetReq)
	s.chAPISessionsKick = make(chan serverAPISessionsKickReq)
	s.done = make(chan struct{})

	s.httpServer = &httpServer{
		http2Address:      s.HTTP2Address,
		http3Address:      s.HTTP3Address,
		serverKey:         s.ServerKey,
		serverCert:        s.ServerCert,
		allowOrigins:      s.AllowOrigins,
		trustedProxies:    s.TrustedProxies,
		udpReadBufferSize: s.UDPReadBufferSize,
		readTimeout:       s.ReadTimeout,
		writeTimeout:      s.WriteTimeout,
		pathManager:       s.PathManager,
		parent:            s,
	}
	err := s.httpServer.initialize()
	if err != nil {
		ctxCancel()
		return err
	}

	s.Log(logger.Info, "started with listeners on %s (TCP/HTTP2), %s (UDP/HTTP3)", s.HTTP2Address, s.HTTP3Address)

	go s.run()

	if !interfaceIsEmpty(s.Metrics) {
		s.Metrics.SetMoQServer(s)
	}

	return nil
}

// Log implements logger.Writer.
func (s *Server) Log(level logger.Level, format string, args ...any) {
	s.Parent.Log(level, "[MoQ] "+format, args...)
}

// Close closes the server.
func (s *Server) Close() {
	s.Log(logger.Info, "closing")

	if !interfaceIsEmpty(s.Metrics) {
		s.Metrics.SetMoQServer(nil)
	}

	s.ctxCancel()
	<-s.done
}

func (s *Server) run() {
	defer close(s.done)

	var wg sync.WaitGroup

outer:
	for {
		select {
		case req := <-s.chNewSession:
			sx := &session{
				wt:          req.wt,
				wg:          &wg,
				pathName:    req.pathName,
				query:       req.query,
				userAgent:   req.userAgent,
				pathManager: s.PathManager,
				parent:      s,
			}
			sx.initialize()
			s.sessions[sx] = struct{}{}
			req.res <- newSessionRes{sx: sx}

		case sx := <-s.chCloseSession:
			delete(s.sessions, sx)

		case req := <-s.chAPISessionsList:
			data := &defs.APIMoQSessionList{
				Items: []defs.APIMoQSession{},
			}
			for sx := range s.sessions {
				data.Items = append(data.Items, sx.apiItem())
			}
			req.res <- serverAPISessionsListRes{data: data}

		case req := <-s.chAPISessionsGet:
			var found *session
			for sx := range s.sessions {
				if sx.uuid == req.uuid {
					found = sx
					break
				}
			}
			if found == nil {
				req.res <- serverAPISessionsGetRes{err: ErrSessionNotFound}
			} else {
				item := found.apiItem()
				req.res <- serverAPISessionsGetRes{data: &item}
			}

		case req := <-s.chAPISessionsKick:
			var found *session
			for sx := range s.sessions {
				if sx.uuid == req.uuid {
					found = sx
					break
				}
			}
			if found == nil {
				req.res <- serverAPISessionsKickRes{err: ErrSessionNotFound}
			} else {
				found.Close()
				req.res <- serverAPISessionsKickRes{}
			}

		case <-s.ctx.Done():
			break outer
		}
	}

	// close sessions before closing UDP packet listener
	for sx := range s.sessions {
		sx.Close()
	}

	s.httpServer.close()

	wg.Wait()
}

func (s *Server) newSession(req newSessionReq) newSessionRes {
	req.res = make(chan newSessionRes)

	select {
	case s.chNewSession <- req:
		return <-req.res
	case <-s.ctx.Done():
		return newSessionRes{err: fmt.Errorf("terminated")}
	}
}

// closeSession is called by session.
func (s *Server) closeSession(sx *session) {
	select {
	case s.chCloseSession <- sx:
	case <-s.ctx.Done():
	}
}

// APISessionsList implements defs.APIMoQServer.
func (s *Server) APISessionsList() (*defs.APIMoQSessionList, error) {
	req := serverAPISessionsListReq{
		res: make(chan serverAPISessionsListRes),
	}
	select {
	case s.chAPISessionsList <- req:
		res := <-req.res
		return res.data, nil
	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}

// APISessionsGet implements defs.APIMoQServer.
func (s *Server) APISessionsGet(id uuid.UUID) (*defs.APIMoQSession, error) {
	req := serverAPISessionsGetReq{
		uuid: id,
		res:  make(chan serverAPISessionsGetRes),
	}
	select {
	case s.chAPISessionsGet <- req:
		res := <-req.res
		return res.data, res.err
	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}

// APISessionsKick implements defs.APIMoQServer.
func (s *Server) APISessionsKick(id uuid.UUID) error {
	req := serverAPISessionsKickReq{
		uuid: id,
		res:  make(chan serverAPISessionsKickRes),
	}
	select {
	case s.chAPISessionsKick <- req:
		res := <-req.res
		return res.err
	case <-s.ctx.Done():
		return fmt.Errorf("terminated")
	}
}
