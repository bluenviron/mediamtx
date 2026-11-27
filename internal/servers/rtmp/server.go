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
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/certloader"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/packetdumper"
	"github.com/bluenviron/mediamtx/internal/protocols/proxy"
	"github.com/bluenviron/mediamtx/internal/restrictnetwork"
)

// ErrConnNotFound is returned when a connection is not found.
var ErrConnNotFound = errors.New("connection not found")

const rejectLogInterval = 10 * time.Second

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

type serverAdmitProxyConnReq struct {
	c   *conn
	ip  net.IP
	res chan bool
}

type serverMetrics interface {
	SetRTMPSServer(defs.APIRTMPServer)
	SetRTMPServer(defs.APIRTMPServer)
}

type serverPathManager interface {
	FindPathConf(req defs.PathFindPathConfReq) (*defs.PathFindPathConfRes, error)
	AddPublisher(req defs.PathAddPublisherReq) (*defs.PathAddPublisherRes, error)
	AddReader(req defs.PathAddReaderReq) (*defs.PathAddReaderRes, error)
}

type serverParent interface {
	logger.Writer
	OnFatalError(context.Context, error)
}

// Server is a RTMP server.
type Server struct {
	Address                      string
	DumpPackets                  bool
	ReadTimeout                  conf.Duration
	WriteTimeout                 conf.Duration
	Encryption                   bool
	ServerCert                   string
	ServerKey                    string
	RTSPAddress                  string
	TrustedProxies               conf.IPNetworks
	MaxUnauthenticatedConns      int
	MaxUnauthenticatedConnsPerIP int
	RunOnConnect                 string
	RunOnConnectRestart          bool
	RunOnDisconnect              string
	ExternalCmdPool              *externalcmd.Pool
	Metrics                      serverMetrics
	PathManager                  serverPathManager
	Parent                       serverParent

	ctx       context.Context
	ctxCancel func()
	wg        sync.WaitGroup
	ln        net.Listener
	conns     map[*conn]struct{}
	loader    *certloader.CertLoader

	unauthConns      int
	unauthConnsPerIP map[string]int
	rejectedConns    atomic.Uint64
	lastRejectLog    time.Time
	rejectedSinceLog uint64

	// in
	chNewConn        chan net.Conn
	chListenError    chan error
	chCloseConn      chan *conn
	chAuthorized     chan *conn
	chAdmitProxyConn chan serverAdmitProxyConnReq
	chAPIConnsList   chan serverAPIConnsListReq
	chAPIConnsGet    chan serverAPIConnsGetReq
	chAPIConnsKick   chan serverAPIConnsKickReq
}

// Initialize initializes the server.
func (s *Server) Initialize() error {
	listen := func(network, address string) (net.Listener, error) {
		ln, err := net.Listen(network, address)
		if err != nil {
			return nil, err
		}

		if s.DumpPackets {
			var proto string
			if s.Encryption {
				proto = "rtmps"
			} else {
				proto = "rtmp"
			}

			ln = &packetdumper.Listener{
				Wrapped: ln,
				Prefix:  proto + "_server_conn",
			}
		}

		if len(s.TrustedProxies) > 0 {
			pl := &proxy.Listener{
				Wrapped:        ln,
				TrustedProxies: s.TrustedProxies,
			}
			pl.Initialize()
			ln = pl
		}

		return ln, nil
	}

	tlsListen := func(network string, laddr string, config *tls.Config) (net.Listener, error) {
		ln, err := listen(network, laddr)
		if err != nil {
			return nil, err
		}

		if s.DumpPackets {
			ln = &packetdumper.TLSListener{
				Wrapped:   ln,
				TLSConfig: config,
			}
		} else {
			ln = tls.NewListener(ln, config)
		}

		return ln, nil
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
		s.ln, err = tlsListen(net, addr, &tls.Config{GetCertificate: s.loader.GetCertificate})
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
	s.unauthConnsPerIP = make(map[string]int)
	s.chNewConn = make(chan net.Conn)
	s.chListenError = make(chan error)
	s.chCloseConn = make(chan *conn)
	s.chAuthorized = make(chan *conn)
	s.chAdmitProxyConn = make(chan serverAdmitProxyConnReq)
	s.chAPIConnsList = make(chan serverAPIConnsListReq)
	s.chAPIConnsGet = make(chan serverAPIConnsGetReq)
	s.chAPIConnsKick = make(chan serverAPIConnsKickReq)

	str := "started with listener on " + s.Address
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
	s.Log(logger.Info, "closing")

	if !interfaceIsEmpty(s.Metrics) {
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

	s.Log(logger.Debug, "closed")
}

func (s *Server) run() {
	defer s.wg.Done()

outer:
	for {
		select {
		case err := <-s.chListenError:
			s.Log(logger.Error, "%s", err)
			s.Parent.OnFatalError(s.ctx, err)
			break outer

		case nconn := <-s.chNewConn:
			if len(s.TrustedProxies) > 0 {
				if s.MaxUnauthenticatedConns != 0 && s.unauthConns >= s.MaxUnauthenticatedConns {
					s.rejectConn(nconn, rawRemoteIP(nconn))
					continue
				}

				s.unauthConns++
				s.newServerConn(nconn, nil, "", true).initialize()
				continue
			}

			tcpAddr, ok := remoteTCPAddr(nconn.RemoteAddr())
			if !ok {
				nconn.Close()
				continue
			}

			admissionKey, ok := s.acquireUnauthenticated(tcpAddr.IP)
			if !ok {
				s.rejectConn(nconn, tcpAddr.IP)
				continue
			}

			c := s.newServerConn(nconn, tcpAddr, admissionKey, false)
			c.initialize()
			s.conns[c] = struct{}{}

		case req := <-s.chAdmitProxyConn:
			if s.MaxUnauthenticatedConnsPerIP != 0 {
				key := req.ip.String()
				if s.unauthConnsPerIP[key] >= s.MaxUnauthenticatedConnsPerIP {
					s.releaseUnauthenticated(req.c)
					s.rejectConn(req.c.nconn, req.ip)
					req.res <- false
					continue
				}
				s.unauthConnsPerIP[key]++
				req.c.admissionKey = key
			}
			s.conns[req.c] = struct{}{}
			req.res <- true

		case c := <-s.chCloseConn:
			delete(s.conns, c)
			s.releaseUnauthenticated(c)

		case c := <-s.chAuthorized:
			s.releaseUnauthenticated(c)

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
			s.releaseUnauthenticated(c)
			c.Close()
			req.res <- serverAPIConnsKickRes{}

		case <-s.ctx.Done():
			break outer
		}
	}

	s.ctxCancel()

	s.ln.Close()
}

func (s *Server) newServerConn(nconn net.Conn, remoteAddr *net.TCPAddr, admissionKey string, waitProxyAddr bool) *conn {
	return &conn{
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
		waitProxyAddr:       waitProxyAddr,
		remoteAddrVal:       remoteAddr,
		unauthenticated:     true,
		admissionKey:        admissionKey,
	}
}

// acquireUnauthenticated reserves a slot for a connection that has not been authorized yet.
func (s *Server) acquireUnauthenticated(ip net.IP) (string, bool) {
	if s.MaxUnauthenticatedConns != 0 && s.unauthConns >= s.MaxUnauthenticatedConns {
		return "", false
	}

	var key string
	if s.MaxUnauthenticatedConnsPerIP != 0 {
		key = ip.String()
		if s.unauthConnsPerIP[key] >= s.MaxUnauthenticatedConnsPerIP {
			return "", false
		}
		s.unauthConnsPerIP[key]++
	}

	s.unauthConns++
	return key, true
}

// releaseUnauthenticated frees the slot of a connection, if it still holds one.
func (s *Server) releaseUnauthenticated(c *conn) {
	if !c.unauthenticated {
		return
	}
	c.unauthenticated = false

	s.unauthConns--

	if c.admissionKey != "" {
		s.unauthConnsPerIP[c.admissionKey]--
		if s.unauthConnsPerIP[c.admissionKey] == 0 {
			delete(s.unauthConnsPerIP, c.admissionKey)
		}
		c.admissionKey = ""
	}
}

// rejectConn closes a connection with a TCP RST, before any TLS handshake or parsing takes place.
func (s *Server) rejectConn(nconn net.Conn, ip net.IP) {
	s.rejectedConns.Add(1)
	s.rejectedSinceLog++

	if tcpConn := unwrapTCPConn(nconn); tcpConn != nil {
		// best effort: if linger can't be set, the connection is closed gracefully.
		_ = tcpConn.SetLinger(0)
	}
	nconn.Close()

	if now := time.Now(); now.Sub(s.lastRejectLog) >= rejectLogInterval {
		s.Log(logger.Warn, "rejected %d connection(s) since the limit of unauthenticated connections "+
			"has been reached (last one from %v)", s.rejectedSinceLog, ip)
		s.lastRejectLog = now
		s.rejectedSinceLog = 0
	}
}

func unwrapTCPConn(nconn net.Conn) *net.TCPConn {
	for {
		switch tconn := nconn.(type) {
		case *net.TCPConn:
			return tconn

		case *tls.Conn:
			nconn = tconn.NetConn()

		case *packetdumper.Conn:
			nconn = tconn.Conn

		case interface{ Raw() net.Conn }: // PROXY protocol
			nconn = tconn.Raw()

		default:
			return nil
		}
	}
}
func remoteTCPAddr(addr net.Addr) (*net.TCPAddr, bool) {
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok || tcpAddr == nil || tcpAddr.IP == nil {
		return nil, false
	}
	return tcpAddr, true
}

func rawRemoteIP(nconn net.Conn) net.IP {
	if tcpConn := unwrapTCPConn(nconn); tcpConn != nil {
		if tcpAddr, ok := remoteTCPAddr(tcpConn.RemoteAddr()); ok {
			return tcpAddr.IP
		}
	}
	if tcpAddr, ok := remoteTCPAddr(nconn.RemoteAddr()); ok {
		return tcpAddr.IP
	}
	return nil
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

// connAuthorized is called by conn.
func (s *Server) connAuthorized(c *conn) {
	select {
	case s.chAuthorized <- c:
	case <-s.ctx.Done():
	}
}

// admitProxyConn completes per-IP admission for a connection behind a trusted proxy.
func (s *Server) admitProxyConn(c *conn, ip net.IP) bool {
	req := serverAdmitProxyConnReq{
		c:   c,
		ip:  ip,
		res: make(chan bool),
	}

	select {
	case s.chAdmitProxyConn <- req:
		return <-req.res
	case <-s.ctx.Done():
		c.nconn.Close()
		return false
	}
}

// listenError is called by rtmpListener.
func (s *Server) listenError(err error) {
	select {
	case s.chListenError <- err:
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

// APIConnsList implements defs.APIRTMPServer.
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

// APIConnsGet implements defs.APIRTMPServer.
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

// RejectedConns implements defs.APIRTMPServer.
func (s *Server) RejectedConns() uint64 {
	return s.rejectedConns.Load()
}

// APIConnsKick implements defs.APIRTMPServer.
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
