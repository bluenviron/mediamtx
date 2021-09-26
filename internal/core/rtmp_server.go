package core

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"sync"

	"github.com/aler9/gortsplib"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type rtmpServerParent interface {
	Log(logger.Level, string, ...interface{})
}

type rtmpServer struct {
	readTimeout         conf.StringDuration
	writeTimeout        conf.StringDuration
	readBufferCount     int
	rtspAddress         string
	runOnConnect        string
	runOnConnectRestart bool
	metrics             *metrics
	pathManager         *pathManager
	parent              rtmpServerParent

	ctx       context.Context
	ctxCancel func()
	wg        sync.WaitGroup
	l         net.Listener
	conns     map[*rtmpConn]struct{}

	// in
	connClose        chan *rtmpConn
	apiRTMPConnsList chan apiRTMPConnsListReq
	apiRTMPConnsKick chan apiRTMPConnsKickReq
}

func newRTMPServer(
	parentCtx context.Context,
	address string,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	readBufferCount int,
	rtspAddress string,
	runOnConnect string,
	runOnConnectRestart bool,
	metrics *metrics,
	pathManager *pathManager,
	parent rtmpServerParent) (*rtmpServer, error) {
	l, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	ctx, ctxCancel := context.WithCancel(parentCtx)

	s := &rtmpServer{
		readTimeout:         readTimeout,
		writeTimeout:        writeTimeout,
		readBufferCount:     readBufferCount,
		rtspAddress:         rtspAddress,
		runOnConnect:        runOnConnect,
		runOnConnectRestart: runOnConnectRestart,
		metrics:             metrics,
		pathManager:         pathManager,
		parent:              parent,
		ctx:                 ctx,
		ctxCancel:           ctxCancel,
		l:                   l,
		conns:               make(map[*rtmpConn]struct{}),
		connClose:           make(chan *rtmpConn),
		apiRTMPConnsList:    make(chan apiRTMPConnsListReq),
		apiRTMPConnsKick:    make(chan apiRTMPConnsKickReq),
	}

	s.Log(logger.Info, "listener opened on %s", address)

	if s.metrics != nil {
		s.metrics.OnRTMPServerSet(s)
	}

	s.wg.Add(1)
	go s.run()

	return s, nil
}

func (s *rtmpServer) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[RTMP] "+format, append([]interface{}{}, args...)...)
}

func (s *rtmpServer) close() {
	s.ctxCancel()
	s.wg.Wait()
	s.Log(logger.Info, "closed")
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
				conn, err := s.l.Accept()
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
			s.Log(logger.Warn, "ERR: %s", err)
			break outer

		case nconn := <-connNew:
			id, _ := s.newConnID()

			c := newRTMPConn(
				s.ctx,
				id,
				s.rtspAddress,
				s.readTimeout,
				s.writeTimeout,
				s.readBufferCount,
				s.runOnConnect,
				s.runOnConnectRestart,
				&s.wg,
				nconn,
				s.pathManager,
				s)
			s.conns[c] = struct{}{}

		case c := <-s.connClose:
			if _, ok := s.conns[c]; !ok {
				continue
			}
			delete(s.conns, c)

		case req := <-s.apiRTMPConnsList:
			data := &apiRTMPConnsListData{
				Items: make(map[string]apiRTMPConnsListItem),
			}

			for c := range s.conns {
				data.Items[c.ID()] = apiRTMPConnsListItem{
					RemoteAddr: c.RemoteAddr().String(),
					State: func() string {
						switch c.safeState() {
						case gortsplib.ServerSessionStateRead:
							return "read"

						case gortsplib.ServerSessionStatePublish:
							return "publish"
						}
						return "idle"
					}(),
				}
			}

			req.Res <- apiRTMPConnsListRes{Data: data}

		case req := <-s.apiRTMPConnsKick:
			res := func() bool {
				for c := range s.conns {
					if c.ID() == req.ID {
						delete(s.conns, c)
						c.Close()
						return true
					}
				}
				return false
			}()
			if res {
				req.Res <- apiRTMPConnsKickRes{}
			} else {
				req.Res <- apiRTMPConnsKickRes{fmt.Errorf("not found")}
			}

		case <-s.ctx.Done():
			break outer
		}
	}

	s.ctxCancel()

	s.l.Close()

	if s.metrics != nil {
		s.metrics.OnRTMPServerSet(s)
	}
}

func (s *rtmpServer) newConnID() (string, error) {
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
			for c := range s.conns {
				if c.ID() == id {
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

// OnConnClose is called by rtmpConn.
func (s *rtmpServer) OnConnClose(c *rtmpConn) {
	select {
	case s.connClose <- c:
	case <-s.ctx.Done():
	}
}

// OnAPIRTMPConnsList is called by api.
func (s *rtmpServer) OnAPIRTMPConnsList(req apiRTMPConnsListReq) apiRTMPConnsListRes {
	req.Res = make(chan apiRTMPConnsListRes)
	select {
	case s.apiRTMPConnsList <- req:
		return <-req.Res
	case <-s.ctx.Done():
		return apiRTMPConnsListRes{Err: fmt.Errorf("terminated")}
	}
}

// OnAPIRTMPConnsKick is called by api.
func (s *rtmpServer) OnAPIRTMPConnsKick(req apiRTMPConnsKickReq) apiRTMPConnsKickRes {
	req.Res = make(chan apiRTMPConnsKickRes)
	select {
	case s.apiRTMPConnsKick <- req:
		return <-req.Res
	case <-s.ctx.Done():
		return apiRTMPConnsKickRes{Err: fmt.Errorf("terminated")}
	}
}
