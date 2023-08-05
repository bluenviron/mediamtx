package core

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/datarhei/gosrt"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
)

func srtMaxPayloadSize(u int) int {
	return ((u - 16) / 188) * 188 // 16 = SRT header, 188 = MPEG-TS packet
}

type srtNewConnReq struct {
	connReq srt.ConnRequest
	res     chan *srtConn
}

type srtServerAPIConnsListRes struct {
	data *apiSRTConnsList
	err  error
}

type srtServerAPIConnsListReq struct {
	res chan srtServerAPIConnsListRes
}

type srtServerAPIConnsGetRes struct {
	data *apiSRTConn
	err  error
}

type srtServerAPIConnsGetReq struct {
	uuid uuid.UUID
	res  chan srtServerAPIConnsGetRes
}

type srtServerAPIConnsKickRes struct {
	err error
}

type srtServerAPIConnsKickReq struct {
	uuid uuid.UUID
	res  chan srtServerAPIConnsKickRes
}

type srtServerParent interface {
	logger.Writer
}

type srtServer struct {
	readTimeout       conf.StringDuration
	writeTimeout      conf.StringDuration
	readBufferCount   int
	udpMaxPayloadSize int
	externalCmdPool   *externalcmd.Pool
	pathManager       *pathManager
	parent            srtServerParent

	ctx       context.Context
	ctxCancel func()
	wg        sync.WaitGroup
	ln        srt.Listener
	conns     map[*srtConn]struct{}

	// in
	chNewConnRequest chan srtNewConnReq
	chAcceptErr      chan error
	chCloseConn      chan *srtConn
	chAPIConnsList   chan srtServerAPIConnsListReq
	chAPIConnsGet    chan srtServerAPIConnsGetReq
	chAPIConnsKick   chan srtServerAPIConnsKickReq
}

func newSRTServer(
	address string,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	readBufferCount int,
	udpMaxPayloadSize int,
	externalCmdPool *externalcmd.Pool,
	pathManager *pathManager,
	parent srtServerParent,
) (*srtServer, error) {
	conf := srt.DefaultConfig()
	conf.ConnectionTimeout = time.Duration(readTimeout)
	conf.PayloadSize = uint32(srtMaxPayloadSize(udpMaxPayloadSize))

	ln, err := srt.Listen("srt", address, conf)
	if err != nil {
		return nil, err
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	s := &srtServer{
		readTimeout:       readTimeout,
		writeTimeout:      writeTimeout,
		readBufferCount:   readBufferCount,
		udpMaxPayloadSize: udpMaxPayloadSize,
		externalCmdPool:   externalCmdPool,
		pathManager:       pathManager,
		parent:            parent,
		ctx:               ctx,
		ctxCancel:         ctxCancel,
		ln:                ln,
		conns:             make(map[*srtConn]struct{}),
		chNewConnRequest:  make(chan srtNewConnReq),
		chAcceptErr:       make(chan error),
		chCloseConn:       make(chan *srtConn),
		chAPIConnsList:    make(chan srtServerAPIConnsListReq),
		chAPIConnsGet:     make(chan srtServerAPIConnsGetReq),
		chAPIConnsKick:    make(chan srtServerAPIConnsKickReq),
	}

	s.Log(logger.Info, "listener opened on "+address+" (UDP)")

	newSRTListener(
		s.ln,
		&s.wg,
		s,
	)

	s.wg.Add(1)
	go s.run()

	return s, nil
}

// Log is the main logging function.
func (s *srtServer) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[SRT] "+format, append([]interface{}{}, args...)...)
}

func (s *srtServer) close() {
	s.Log(logger.Info, "listener is closing")
	s.ctxCancel()
	s.wg.Wait()
}

func (s *srtServer) run() {
	defer s.wg.Done()

outer:
	for {
		select {
		case err := <-s.chAcceptErr:
			s.Log(logger.Error, "%s", err)
			break outer

		case req := <-s.chNewConnRequest:
			c := newSRTConn(
				s.ctx,
				s.readTimeout,
				s.writeTimeout,
				s.readBufferCount,
				s.udpMaxPayloadSize,
				req.connReq,
				&s.wg,
				s.externalCmdPool,
				s.pathManager,
				s)
			s.conns[c] = struct{}{}
			req.res <- c

		case c := <-s.chCloseConn:
			delete(s.conns, c)

		case req := <-s.chAPIConnsList:
			data := &apiSRTConnsList{
				Items: []*apiSRTConn{},
			}

			for c := range s.conns {
				data.Items = append(data.Items, c.apiItem())
			}

			sort.Slice(data.Items, func(i, j int) bool {
				return data.Items[i].Created.Before(data.Items[j].Created)
			})

			req.res <- srtServerAPIConnsListRes{data: data}

		case req := <-s.chAPIConnsGet:
			c := s.findConnByUUID(req.uuid)
			if c == nil {
				req.res <- srtServerAPIConnsGetRes{err: errAPINotFound}
				continue
			}

			req.res <- srtServerAPIConnsGetRes{data: c.apiItem()}

		case req := <-s.chAPIConnsKick:
			c := s.findConnByUUID(req.uuid)
			if c == nil {
				req.res <- srtServerAPIConnsKickRes{err: errAPINotFound}
				continue
			}

			delete(s.conns, c)
			c.close()
			req.res <- srtServerAPIConnsKickRes{}

		case <-s.ctx.Done():
			break outer
		}
	}

	s.ctxCancel()

	s.ln.Close()
}

func (s *srtServer) findConnByUUID(uuid uuid.UUID) *srtConn {
	for sx := range s.conns {
		if sx.uuid == uuid {
			return sx
		}
	}
	return nil
}

// newConnRequest is called by srtListener.
func (s *srtServer) newConnRequest(connReq srt.ConnRequest) *srtConn {
	req := srtNewConnReq{
		connReq: connReq,
		res:     make(chan *srtConn),
	}

	select {
	case s.chNewConnRequest <- req:
		c := <-req.res

		return c.new(req)

	case <-s.ctx.Done():
		return nil
	}
}

// acceptError is called by srtListener.
func (s *srtServer) acceptError(err error) {
	select {
	case s.chAcceptErr <- err:
	case <-s.ctx.Done():
	}
}

// closeConn is called by srtConn.
func (s *srtServer) closeConn(c *srtConn) {
	select {
	case s.chCloseConn <- c:
	case <-s.ctx.Done():
	}
}

// apiConnsList is called by api.
func (s *srtServer) apiConnsList() (*apiSRTConnsList, error) {
	req := srtServerAPIConnsListReq{
		res: make(chan srtServerAPIConnsListRes),
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
func (s *srtServer) apiConnsGet(uuid uuid.UUID) (*apiSRTConn, error) {
	req := srtServerAPIConnsGetReq{
		uuid: uuid,
		res:  make(chan srtServerAPIConnsGetRes),
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
func (s *srtServer) apiConnsKick(uuid uuid.UUID) error {
	req := srtServerAPIConnsKickReq{
		uuid: uuid,
		res:  make(chan srtServerAPIConnsKickRes),
	}

	select {
	case s.chAPIConnsKick <- req:
		res := <-req.res
		return res.err

	case <-s.ctx.Done():
		return fmt.Errorf("terminated")
	}
}
