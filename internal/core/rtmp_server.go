package core

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/externalcmd"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type rtmpServerAPIConnsListItem struct {
	Created       time.Time `json:"created"`
	RemoteAddr    string    `json:"remoteAddr"`
	State         string    `json:"state"`
	BytesReceived uint64    `json:"bytesReceived"`
	BytesSent     uint64    `json:"bytesSent"`
}

type rtmpServerAPIConnsListData struct {
	Items map[string]rtmpServerAPIConnsListItem `json:"items"`
}

type rtmpServerAPIConnsListRes struct {
	data *rtmpServerAPIConnsListData
	err  error
}

type rtmpServerAPIConnsListReq struct {
	res chan rtmpServerAPIConnsListRes
}

type rtmpServerAPIConnsKickRes struct {
	err error
}

type rtmpServerAPIConnsKickReq struct {
	id  string
	res chan rtmpServerAPIConnsKickRes
}

type rtmpServerParent interface {
	Log(logger.Level, string, ...interface{})
}

type rtmpServer struct {
	externalAuthenticationURL string
	readTimeout               conf.StringDuration
	writeTimeout              conf.StringDuration
	readBufferCount           int
	isTLS                     bool
	rtspAddress               string
	runOnConnect              string
	runOnConnectRestart       bool
	externalCmdPool           *externalcmd.Pool
	metrics                   *metrics
	pathManager               *pathManager
	parent                    rtmpServerParent

	ctx       context.Context
	ctxCancel func()
	wg        sync.WaitGroup
	ln        net.Listener
	conns     map[*rtmpConn]struct{}

	// in
	chConnClose    chan *rtmpConn
	chAPIConnsList chan rtmpServerAPIConnsListReq
	chAPIConnsKick chan rtmpServerAPIConnsKickReq
}

func newRTMPServer(
	parentCtx context.Context,
	externalAuthenticationURL string,
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
			return net.Listen("tcp", address)
		}

		cert, err := tls.LoadX509KeyPair(serverCert, serverKey)
		if err != nil {
			return nil, err
		}

		return tls.Listen("tcp", address, &tls.Config{Certificates: []tls.Certificate{cert}})
	}()
	if err != nil {
		return nil, err
	}

	ctx, ctxCancel := context.WithCancel(parentCtx)

	s := &rtmpServer{
		externalAuthenticationURL: externalAuthenticationURL,
		readTimeout:               readTimeout,
		writeTimeout:              writeTimeout,
		readBufferCount:           readBufferCount,
		rtspAddress:               rtspAddress,
		runOnConnect:              runOnConnect,
		runOnConnectRestart:       runOnConnectRestart,
		isTLS:                     isTLS,
		externalCmdPool:           externalCmdPool,
		metrics:                   metrics,
		pathManager:               pathManager,
		parent:                    parent,
		ctx:                       ctx,
		ctxCancel:                 ctxCancel,
		ln:                        ln,
		conns:                     make(map[*rtmpConn]struct{}),
		chConnClose:               make(chan *rtmpConn),
		chAPIConnsList:            make(chan rtmpServerAPIConnsListReq),
		chAPIConnsKick:            make(chan rtmpServerAPIConnsKickReq),
	}

	s.log(logger.Info, "listener opened on %s", address)

	if s.metrics != nil {
		s.metrics.rtmpServerSet(s)
	}

	s.wg.Add(1)
	go s.run()

	return s, nil
}

func (s *rtmpServer) log(level logger.Level, format string, args ...interface{}) {
	label := func() string {
		if s.isTLS {
			return "RTMPS"
		}
		return "RTMP"
	}()
	s.parent.Log(level, "[%s] "+format, append([]interface{}{label}, args...)...)
}

func (s *rtmpServer) close() {
	s.log(logger.Info, "listener is closing")
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
			s.log(logger.Error, "%s", err)
			break outer

		case nconn := <-connNew:
			c := newRTMPConn(
				s.ctx,
				s.isTLS,
				s.externalAuthenticationURL,
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
			data := &rtmpServerAPIConnsListData{
				Items: make(map[string]rtmpServerAPIConnsListItem),
			}

			for c := range s.conns {
				data.Items[c.uuid.String()] = rtmpServerAPIConnsListItem{
					Created:    c.created,
					RemoteAddr: c.remoteAddr().String(),
					State: func() string {
						switch c.safeState() {
						case rtmpConnStateRead:
							return "read"

						case rtmpConnStatePublish:
							return "publish"
						}
						return "idle"
					}(),
					BytesReceived: c.conn.BytesReceived(),
					BytesSent:     c.conn.BytesSent(),
				}
			}

			req.res <- rtmpServerAPIConnsListRes{data: data}

		case req := <-s.chAPIConnsKick:
			res := func() bool {
				for c := range s.conns {
					if c.uuid.String() == req.id {
						delete(s.conns, c)
						c.close()
						return true
					}
				}
				return false
			}()
			if res {
				req.res <- rtmpServerAPIConnsKickRes{}
			} else {
				req.res <- rtmpServerAPIConnsKickRes{fmt.Errorf("not found")}
			}

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

// connClose is called by rtmpConn.
func (s *rtmpServer) connClose(c *rtmpConn) {
	select {
	case s.chConnClose <- c:
	case <-s.ctx.Done():
	}
}

// apiConnsList is called by api.
func (s *rtmpServer) apiConnsList() rtmpServerAPIConnsListRes {
	req := rtmpServerAPIConnsListReq{
		res: make(chan rtmpServerAPIConnsListRes),
	}

	select {
	case s.chAPIConnsList <- req:
		return <-req.res

	case <-s.ctx.Done():
		return rtmpServerAPIConnsListRes{err: fmt.Errorf("terminated")}
	}
}

// apiConnsKick is called by api.
func (s *rtmpServer) apiConnsKick(id string) rtmpServerAPIConnsKickRes {
	req := rtmpServerAPIConnsKickReq{
		id:  id,
		res: make(chan rtmpServerAPIConnsKickRes),
	}

	select {
	case s.chAPIConnsKick <- req:
		return <-req.res

	case <-s.ctx.Done():
		return rtmpServerAPIConnsKickRes{err: fmt.Errorf("terminated")}
	}
}
