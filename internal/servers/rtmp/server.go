// Package rtmp contains a RTMP server.
package rtmp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"reflect"
	"sort"
	"sync"

	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/certloader"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/packetdumper"
	"github.com/bluenviron/mediamtx/internal/restrictnetwork"
)

// ErrConnNotFound is returned when a connection is not found.
var ErrConnNotFound = errors.New("connection not found")

func interfaceIsEmpty(i any) bool {
	return reflect.ValueOf(i).Kind() != reflect.Pointer || reflect.ValueOf(i).IsNil()
}

type serverAPIConnsListRes struct {
	data *defs.APIRTMPConnList
	err  error
}

type serverAPIConnsListReq struct {
	res chan serverAPIConnsListRes
}

type serverAPIConnsGetRes struct {
	data *defs.APIRTMPConn
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
	SetRTMPSServer(defs.APIRTMPServer)
	SetRTMPServer(defs.APIRTMPServer)
}

type serverPathManager interface {
	AddPublisher(req defs.PathAddPublisherReq) (*defs.PathAddPublisherRes, error)
	AddReader(req defs.PathAddReaderReq) (*defs.PathAddReaderRes, error)
}

type serverParent interface {
	logger.Writer
}

// Server is a RTMP server.
type Server struct {
	Address             string
	DumpPackets         bool
	ReadTimeout         conf.Duration
	WriteTimeout        conf.Duration
	Encryption          bool
	ServerCert          string
	ServerKey           string
	RTSPAddress         string
	RunOnConnect        string
	RunOnConnectRestart bool
	RunOnDisconnect     string
	ExternalCmdPool     *externalcmd.Pool
	Metrics             serverMetrics
	PathManager         serverPathManager
	Parent              serverParent

	ctx       context.Context
	ctxCancel func()
	wg        sync.WaitGroup
	ln        net.Listener
	conns     map[*conn]struct{}
	loader    *certloader.CertLoader

	// in
	chNewConn      chan net.Conn
	chAcceptErr    chan error
	chCloseConn    chan *conn
	chAPIConnsList chan serverAPIConnsListReq
	chAPIConnsGet  chan serverAPIConnsGetReq
	chAPIConnsKick chan serverAPIConnsKickReq
}

// Initialize initializes the server.
func (s *Server) Initialize() error {
	var listen func(network string, address string) (net.Listener, error)
	var tlsListen func(network string, laddr string, config *tls.Config) (net.Listener, error)

	if s.DumpPackets {
		var proto string
		if s.Encryption {
			proto = "rtmps"
		} else {
			proto = "rtmp"
		}

		listen = (&packetdumper.Listen{
			Prefix: proto + "_server_conn",
		}).Do

		tlsListen = (&packetdumper.TLSListen{
			Listen: listen,
		}).Do
	} else {
		listen = net.Listen
		tlsListen = tls.Listen
	}

	if s.Encryption {
		s.loader = &certloader.CertLoader{
			CertPath: s.ServerCert,
			KeyPath:  s.ServerKey,
			Parent:   s.Parent,
		}
		err := s.loader.Initialize()
		if err != nil {
			return err
		}

		net, addr := restrictnetwork.Restrict("tcp", s.Address)
		s.ln, err = tlsListen(net, addr, &tls.Config{GetCertificate: s.loader.GetCertificate()})
		if err != nil {
			return err
		}
	} else {
		var err error
		s.ln, err = listen(restrictnetwork.Restrict("tcp", s.Address))
		if err != nil {
			return err
		}
	}

	s.ctx, s.ctxCancel = context.WithCancel(context.Background())

	s.conns = make(map[*conn]struct{})
	s.chNewConn = make(chan net.Conn)
	s.chAcceptErr = make(chan error)
	s.chCloseConn = make(chan *conn)
	s.chAPIConnsList = make(chan serverAPIConnsListReq)
	s.chAPIConnsGet = make(chan serverAPIConnsGetReq)
	s.chAPIConnsKick = make(chan serverAPIConnsKickReq)

	str := "listener opened on " + s.Address
	if s.Encryption {
		str += " (TCP/RTMPS)"
	} else {
		str += " (TCP/RTMP)"
	}
	s.Log(logger.Info, str)

	l := &listener{
		ln:     s.ln,
		wg:     &s.wg,
		parent: s,
	}
	l.initialize()

	s.wg.Add(1)
	go s.run()

	if !interfaceIsEmpty(s.Metrics) {
		if s.Encryption {
			s.Metrics.SetRTMPSServer(s)
		} else {
			s.Metrics.SetRTMPServer(s)
		}
	}

	return nil
}

// Log implements logger.Writer.
func (s *Server) Log(level logger.Level, format string, args ...any) {
	label := func() string {
		if s.Encryption {
			return "RTMPS"
		}
		return "RTMP"
	}()
	s.Parent.Log(level, "[%s] "+format, append([]any{label}, args...)...)
}

// Close closes the server.
func (s *Server) Close() {
	s.Log(logger.Info, "listener is closing")

	if !interfaceIsEmpty((s.Metrics)) {
		if s.Encryption {
			s.Metrics.SetRTMPSServer(nil)
		} else {
			s.Metrics.SetRTMPServer(nil)
		}
	}

	s.ctxCancel()
	s.wg.Wait()

	if s.loader != nil {
		s.loader.Close()
	}
}

func (s *Server) run() {
	defer s.wg.Done()

outer:
	for {
		select {
		case err := <-s.chAcceptErr:
			s.Log(logger.Error, "%s", err)
			break outer

		case nconn := <-s.chNewConn:
			c := &conn{
				parentCtx:           s.ctx,
				encryption:          s.Encryption,
				rtspAddress:         s.RTSPAddress,
				readTimeout:         s.ReadTimeout,
				writeTimeout:        s.WriteTimeout,
				runOnConnect:        s.RunOnConnect,
				runOnConnectRestart: s.RunOnConnectRestart,
				runOnDisconnect:     s.RunOnDisconnect,
				wg:                  &s.wg,
				nconn:               nconn,
				externalCmdPool:     s.ExternalCmdPool,
				pathManager:         s.PathManager,
				parent:              s,
			}
			c.initialize()
			s.conns[c] = struct{}{}

		case c := <-s.chCloseConn:
			delete(s.conns, c)

		case req := <-s.chAPIConnsList:
			data := &defs.APIRTMPConnList{
				Items: []defs.APIRTMPConn{},
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

	s.ln.Close()
}

func (s *Server) findConnByUUID(uuid uuid.UUID) *conn {
	for c := range s.conns {
		if c.uuid == uuid {
			return c
		}
	}
	return nil
}

// newConn is called by rtmpListener.
func (s *Server) newConn(conn net.Conn) {
	select {
	case s.chNewConn <- conn:
	case <-s.ctx.Done():
		conn.Close()
	}
}

// acceptError is called by rtmpListener.
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

// APIConnsList is called by api.
func (s *Server) APIConnsList() (*defs.APIRTMPConnList, error) {
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

// APIConnsGet is called by api.
func (s *Server) APIConnsGet(uuid uuid.UUID) (*defs.APIRTMPConn, error) {
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

// APIConnsKick is called by api.
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
