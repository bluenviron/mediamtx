// Package srt contains a SRT server.
package srt

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"sync"
	"time"

	srt "github.com/datarhei/gosrt"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// srtListen is a package-level indirection over srt.Listen so that tests
// can substitute a fake listener implementation.
var srtListen = srt.Listen

// Listener restart backoff parameters. Variables (rather than constants) so
// that tests can override them without changing production behavior.
var (
	listenerRestartBaseDelay = 500 * time.Millisecond
	listenerRestartMaxDelay  = 30 * time.Second
)

// ErrConnNotFound is returned when a connection is not found.
var ErrConnNotFound = errors.New("connection not found")

func interfaceIsEmpty(i any) bool {
	return reflect.ValueOf(i).Kind() != reflect.Pointer || reflect.ValueOf(i).IsNil()
}

func srtMaxPayloadSize(u int) int {
	return ((u - 16) / 188) * 188 // 16 = SRT header, 188 = MPEG-TS packet
}

type serverAPIConnsListRes struct {
	data *defs.APISRTConnList
	err  error
}

type serverAPIConnsListReq struct {
	res chan serverAPIConnsListRes
}

type serverAPIConnsGetRes struct {
	data *defs.APISRTConn
	err  error
}

type serverAPIConnsGetReq struct {
	uuid uuid.UUID
	res  chan serverAPIConnsGetRes
}

type serverAPIConnsKickRes struct {
	err error
}

type serverAPIConnsKickReq struct {
	uuid uuid.UUID
	res  chan serverAPIConnsKickRes
}

type serverMetrics interface {
	SetSRTServer(defs.APISRTServer)
}

type serverPathManager interface {
	FindPathConf(req defs.PathFindPathConfReq) (*defs.PathFindPathConfRes, error)
	AddPublisher(req defs.PathAddPublisherReq) (*defs.PathAddPublisherRes, error)
	AddReader(req defs.PathAddReaderReq) (*defs.PathAddReaderRes, error)
}

type serverParent interface {
	logger.Writer
}

// Server is a SRT server.
type Server struct {
	Address             string
	RTSPAddress         string
	ReadTimeout         conf.Duration
	WriteTimeout        conf.Duration
	UDPMaxPayloadSize   int
	RunOnConnect        string
	RunOnConnectRestart bool
	RunOnDisconnect     string
	ExternalCmdPool     *externalcmd.Pool
	Metrics             serverMetrics
	PathManager         serverPathManager
	Parent              serverParent

	ctx          context.Context
	ctxCancel    func()
	wg           sync.WaitGroup
	ln           srt.Listener
	listenerConf srt.Config
	conns        map[*conn]struct{}

	// in
	chNewConnRequest chan srt.ConnRequest
	chAcceptErr      chan error
	chCloseConn      chan *conn
	chAPIConnsList   chan serverAPIConnsListReq
	chAPIConnsGet    chan serverAPIConnsGetReq
	chAPIConnsKick   chan serverAPIConnsKickReq
}

// Initialize initializes the server.
func (s *Server) Initialize() error {
	s.listenerConf = srt.DefaultConfig()
	s.listenerConf.ConnectionTimeout = time.Duration(s.ReadTimeout)
	s.listenerConf.PeerIdleTimeout = time.Duration(s.ReadTimeout)
	s.listenerConf.PayloadSize = uint32(srtMaxPayloadSize(s.UDPMaxPayloadSize))

	var err error
	s.ln, err = srtListen("srt", s.Address, s.listenerConf)
	if err != nil {
		return err
	}

	s.ctx, s.ctxCancel = context.WithCancel(context.Background())

	s.conns = make(map[*conn]struct{})
	s.chNewConnRequest = make(chan srt.ConnRequest)
	s.chAcceptErr = make(chan error)
	s.chCloseConn = make(chan *conn)
	s.chAPIConnsList = make(chan serverAPIConnsListReq)
	s.chAPIConnsGet = make(chan serverAPIConnsGetReq)
	s.chAPIConnsKick = make(chan serverAPIConnsKickReq)

	s.Log(logger.Info, "listener opened on "+s.Address+" (UDP)")

	l := &listener{
		ln:     s.ln,
		wg:     &s.wg,
		parent: s,
	}
	l.initialize()

	s.wg.Add(1)
	go s.run()

	if !interfaceIsEmpty(s.Metrics) {
		s.Metrics.SetSRTServer(s)
	}

	return nil
}

// Log implements logger.Writer.
func (s *Server) Log(level logger.Level, format string, args ...any) {
	s.Parent.Log(level, "[SRT] "+format, args...)
}

// Close closes the server.
func (s *Server) Close() {
	s.Log(logger.Info, "listener is closing")

	if !interfaceIsEmpty(s.Metrics) {
		s.Metrics.SetSRTServer(nil)
	}

	s.ctxCancel()
	s.wg.Wait()
}

func (s *Server) run() {
	defer s.wg.Done()

outer:
	for {
		select {
		case err := <-s.chAcceptErr:
			// ErrListenerClosed is the normal signal emitted when we Close()
			// the listener ourselves during shutdown.
			if errors.Is(err, srt.ErrListenerClosed) {
				break outer
			}
			s.Log(logger.Warn, "listener failed: %s; attempting restart", err)
			if rerr := s.restartListener(); rerr != nil {
				s.Log(logger.Error, "listener restart aborted: %s", rerr)
				break outer
			}

		case req := <-s.chNewConnRequest:
			c := &conn{
				parentCtx:           s.ctx,
				rtspAddress:         s.RTSPAddress,
				readTimeout:         s.ReadTimeout,
				writeTimeout:        s.WriteTimeout,
				udpMaxPayloadSize:   s.UDPMaxPayloadSize,
				connReq:             req,
				runOnConnect:        s.RunOnConnect,
				runOnConnectRestart: s.RunOnConnectRestart,
				runOnDisconnect:     s.RunOnDisconnect,
				wg:                  &s.wg,
				externalCmdPool:     s.ExternalCmdPool,
				pathManager:         s.PathManager,
				parent:              s,
			}
			c.initialize()
			s.conns[c] = struct{}{}

		case c := <-s.chCloseConn:
			delete(s.conns, c)

		case req := <-s.chAPIConnsList:
			data := &defs.APISRTConnList{
				Items: []defs.APISRTConn{},
			}

			for c := range s.conns {
				data.Items = append(data.Items, *c.apiItem())
			}

			sort.Slice(data.Items, func(i, j int) bool {
				return data.Items[i].Created.Before(data.Items[j].Created)
			})

			req.res <- serverAPIConnsListRes{data: data}

		case req := <-s.chAPIConnsGet:
			c := s.findConnByUUID(req.uuid)
			if c == nil {
				req.res <- serverAPIConnsGetRes{err: ErrConnNotFound}
				continue
			}

			req.res <- serverAPIConnsGetRes{data: c.apiItem()}

		case req := <-s.chAPIConnsKick:
			c := s.findConnByUUID(req.uuid)
			if c == nil {
				req.res <- serverAPIConnsKickRes{err: ErrConnNotFound}
				continue
			}

			delete(s.conns, c)
			c.Close()
			req.res <- serverAPIConnsKickRes{}

		case <-s.ctx.Done():
			break outer
		}
	}

	s.ctxCancel()

	if s.ln != nil {
		s.ln.Close()
	}
}

// restartListener disposes of the dead gosrt listener and re-creates a new
// one with the same configuration, using bounded exponential backoff with
// jitter. It returns nil on success, or a non-nil error when the server
// context has been cancelled (i.e. the server is shutting down).
//
// Existing live *conn instances and the server goroutine are untouched;
// only the accept side is recreated.
func (s *Server) restartListener() error {
	if s.ln != nil {
		s.ln.Close()
		s.ln = nil
	}

	delay := listenerRestartBaseDelay
	attempt := 0

	for {
		attempt++

		// jitter: +/- 25% of the current delay
		jitter := time.Duration(rand.Int63n(int64(delay/2))) - delay/4
		wait := delay + jitter

		select {
		case <-s.ctx.Done():
			return fmt.Errorf("server is closing")
		case <-time.After(wait):
		}

		ln, err := srtListen("srt", s.Address, s.listenerConf)
		if err != nil {
			s.Log(logger.Warn, "listener restart attempt %d failed: %s", attempt, err)
			delay *= 2
			if delay > listenerRestartMaxDelay {
				delay = listenerRestartMaxDelay
			}
			continue
		}

		s.ln = ln
		s.Log(logger.Info, "listener restarted on %s after %d attempt(s)", s.Address, attempt)

		l := &listener{
			ln:     s.ln,
			wg:     &s.wg,
			parent: s,
		}
		l.initialize()
		return nil
	}
}

func (s *Server) findConnByUUID(uuid uuid.UUID) *conn {
	for sx := range s.conns {
		if sx.uuid == uuid {
			return sx
		}
	}
	return nil
}

// newConnRequest is called by srtListener.
func (s *Server) newConnRequest(connReq srt.ConnRequest) {
	select {
	case s.chNewConnRequest <- connReq:
	case <-s.ctx.Done():
		connReq.Reject(srt.REJ_CLOSE)
	}
}

// acceptError is called by srtListener.
func (s *Server) acceptError(err error) {
	select {
	case s.chAcceptErr <- err:
	case <-s.ctx.Done():
	}
}

// closeConn is called by conn.
func (s *Server) closeConn(c *conn) {
	select {
	case s.chCloseConn <- c:
	case <-s.ctx.Done():
	}
}

// APIConnsList implements defs.APISRTServer.
func (s *Server) APIConnsList() (*defs.APISRTConnList, error) {
	req := serverAPIConnsListReq{
		res: make(chan serverAPIConnsListRes),
	}

	select {
	case s.chAPIConnsList <- req:
		res := <-req.res
		return res.data, res.err

	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}

// APIConnsGet implements defs.APISRTServer.
func (s *Server) APIConnsGet(uuid uuid.UUID) (*defs.APISRTConn, error) {
	req := serverAPIConnsGetReq{
		uuid: uuid,
		res:  make(chan serverAPIConnsGetRes),
	}

	select {
	case s.chAPIConnsGet <- req:
		res := <-req.res
		return res.data, res.err

	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}

// APIConnsKick implements defs.APISRTServer.
func (s *Server) APIConnsKick(uuid uuid.UUID) error {
	req := serverAPIConnsKickReq{
		uuid: uuid,
		res:  make(chan serverAPIConnsKickRes),
	}

	select {
	case s.chAPIConnsKick <- req:
		res := <-req.res
		return res.err

	case <-s.ctx.Done():
		return fmt.Errorf("terminated")
	}
}
