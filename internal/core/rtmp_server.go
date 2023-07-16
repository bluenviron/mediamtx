package core

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sort"
	"sync"

	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
)

type rtmpServerAPIConnsListRes struct {
	data *apiRTMPConnsList
	err  error
}

type rtmpServerAPIConnsListReq struct {
	res chan rtmpServerAPIConnsListRes
}

type rtmpServerAPIConnsGetRes struct {
	data *apiRTMPConn
	err  error
}

type rtmpServerAPIConnsGetReq struct {
	uuid uuid.UUID
	res  chan rtmpServerAPIConnsGetRes
}

type rtmpServerAPIConnsKickRes struct {
	err error
}

type rtmpServerAPIConnsKickReq struct {
	uuid uuid.UUID
	res  chan rtmpServerAPIConnsKickRes
}

type rtmpServerParent interface {
	logger.Writer
}

type rtmpServer struct {
	readTimeout         conf.StringDuration
	writeTimeout        conf.StringDuration
	readBufferCount     int
	isTLS               bool
	rtspAddress         string
	runOnConnect        string
	runOnConnectRestart bool
	externalCmdPool     *externalcmd.Pool
	metrics             *metrics
	pathManager         *pathManager
	parent              rtmpServerParent

	ctx       context.Context
	ctxCancel func()
	wg        sync.WaitGroup
	ln        net.Listener
	conns     map[*rtmpConn]struct{}

	// in
	chConnClose    chan *rtmpConn
	chAPIConnsList chan rtmpServerAPIConnsListReq
	chAPIConnsGet  chan rtmpServerAPIConnsGetReq
	chAPIConnsKick chan rtmpServerAPIConnsKickReq
}

func newRTMPServer(
	address string,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	readBufferCount int,
	isTLS bool,
	serverCert string,
	serverKey string,
	rtspAddress string,
	runOnConnect string,
	runOnConnectRestart bool,
	externalCmdPool *externalcmd.Pool,
	metrics *metrics,
	pathManager *pathManager,
	parent rtmpServerParent,
) (*rtmpServer, error) {
	ln, err := func() (net.Listener, error) {
		if !isTLS {
			return net.Listen(restrictNetwork("tcp", address))
		}

		cert, err := tls.LoadX509KeyPair(serverCert, serverKey)
		if err != nil {
			return nil, err
		}

		network, address := restrictNetwork("tcp", address)
		return tls.Listen(network, address, &tls.Config{Certificates: []tls.Certificate{cert}})
	}()
	if err != nil {
		return nil, err
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	s := &rtmpServer{
		readTimeout:         readTimeout,
		writeTimeout:        writeTimeout,
		readBufferCount:     readBufferCount,
		rtspAddress:         rtspAddress,
		runOnConnect:        runOnConnect,
		runOnConnectRestart: runOnConnectRestart,
		isTLS:               isTLS,
		externalCmdPool:     externalCmdPool,
		metrics:             metrics,
		pathManager:         pathManager,
		parent:              parent,
		ctx:                 ctx,
		ctxCancel:           ctxCancel,
		ln:                  ln,
		conns:               make(map[*rtmpConn]struct{}),
		chConnClose:         make(chan *rtmpConn),
		chAPIConnsList:      make(chan rtmpServerAPIConnsListReq),
		chAPIConnsGet:       make(chan rtmpServerAPIConnsGetReq),
		chAPIConnsKick:      make(chan rtmpServerAPIConnsKickReq),
	}

	s.Log(logger.Info, "listener opened on %s", address)

	if s.metrics != nil {
		s.metrics.rtmpServerSet(s)
	}

	s.wg.Add(1)
	go s.run()

	return s, nil
}

func (s *rtmpServer) Log(level logger.Level, format string, args ...interface{}) {
	label := func() string {
		if s.isTLS {
			return "RTMPS"
		}
		return "RTMP"
	}()
	s.parent.Log(level, "[%s] "+format, append([]interface{}{label}, args...)...)
}

func (s *rtmpServer) close() {
	s.Log(logger.Info, "listener is closing")
	s.ctxCancel()
	s.wg.Wait()
}

func (s *rtmpServer) run() {
	defer s.wg.Done()

	s.wg.Add(1)
	connNew := make(chan net.Conn)
	acceptErr := make(chan error)
	go func() {
		defer s.wg.Done()
		err := func() error {
			for {
				conn, err := s.ln.Accept()
				if err != nil {
					return err
				}

				select {
				case connNew <- conn:
				case <-s.ctx.Done():
					conn.Close()
				}
			}
		}()

		select {
		case acceptErr <- err:
		case <-s.ctx.Done():
		}
	}()

outer:
	for {
		select {
		case err := <-acceptErr:
			s.Log(logger.Error, "%s", err)
			break outer

		case nconn := <-connNew:
			c := newRTMPConn(
				s.ctx,
				s.isTLS,
				s.rtspAddress,
				s.readTimeout,
				s.writeTimeout,
				s.readBufferCount,
				s.runOnConnect,
				s.runOnConnectRestart,
				&s.wg,
				nconn,
				s.externalCmdPool,
				s.pathManager,
				s)
			s.conns[c] = struct{}{}

		case c := <-s.chConnClose:
			delete(s.conns, c)

		case req := <-s.chAPIConnsList:
			data := &apiRTMPConnsList{
				Items: []*apiRTMPConn{},
			}

			for c := range s.conns {
				data.Items = append(data.Items, c.apiItem())
			}

			sort.Slice(data.Items, func(i, j int) bool {
				return data.Items[i].Created.Before(data.Items[j].Created)
			})

			req.res <- rtmpServerAPIConnsListRes{data: data}

		case req := <-s.chAPIConnsGet:
			c := s.findConnByUUID(req.uuid)
			if c == nil {
				req.res <- rtmpServerAPIConnsGetRes{err: errAPINotFound}
				continue
			}

			req.res <- rtmpServerAPIConnsGetRes{data: c.apiItem()}

		case req := <-s.chAPIConnsKick:
			c := s.findConnByUUID(req.uuid)
			if c == nil {
				req.res <- rtmpServerAPIConnsKickRes{err: errAPINotFound}
				continue
			}

			delete(s.conns, c)
			c.close()
			req.res <- rtmpServerAPIConnsKickRes{}

		case <-s.ctx.Done():
			break outer
		}
	}

	s.ctxCancel()

	s.ln.Close()

	if s.metrics != nil {
		s.metrics.rtmpServerSet(s)
	}
}

func (s *rtmpServer) findConnByUUID(uuid uuid.UUID) *rtmpConn {
	for c := range s.conns {
		if c.uuid == uuid {
			return c
		}
	}
	return nil
}

// connClose is called by rtmpConn.
func (s *rtmpServer) connClose(c *rtmpConn) {
	select {
	case s.chConnClose <- c:
	case <-s.ctx.Done():
	}
}

// apiConnsList is called by api.
func (s *rtmpServer) apiConnsList() (*apiRTMPConnsList, error) {
	req := rtmpServerAPIConnsListReq{
		res: make(chan rtmpServerAPIConnsListRes),
	}

	select {
	case s.chAPIConnsList <- req:
		res := <-req.res
		return res.data, res.err

	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}

// apiConnsGet is called by api.
func (s *rtmpServer) apiConnsGet(uuid uuid.UUID) (*apiRTMPConn, error) {
	req := rtmpServerAPIConnsGetReq{
		uuid: uuid,
		res:  make(chan rtmpServerAPIConnsGetRes),
	}

	select {
	case s.chAPIConnsGet <- req:
		res := <-req.res
		return res.data, res.err

	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}

// apiConnsKick is called by api.
func (s *rtmpServer) apiConnsKick(uuid uuid.UUID) error {
	req := rtmpServerAPIConnsKickReq{
		uuid: uuid,
		res:  make(chan rtmpServerAPIConnsKickRes),
	}

	select {
	case s.chAPIConnsKick <- req:
		res := <-req.res
		return res.err

	case <-s.ctx.Done():
		return fmt.Errorf("terminated")
	}
}
