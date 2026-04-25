// Package hls contains a HLS server.
package hls

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/google/uuid"
)

// ErrMuxerNotFound is returned when a muxer is not found.
var ErrMuxerNotFound = errors.New("muxer not found")

// ErrSessionNotFound is returned when a session is not found.
var ErrSessionNotFound = errors.New("session not found")

func interfaceIsEmpty(i any) bool {
	return reflect.ValueOf(i).Kind() != reflect.Pointer || reflect.ValueOf(i).IsNil()
}

type serverGetMuxerRes struct {
	muxer *muxer
	err   error
}

type serverGetMuxerReq struct {
	path           string
	create         bool
	remoteAddr     string // only if create == true
	query          string // only if create == true
	sourceOnDemand bool   // only if create == true
	res            chan serverGetMuxerRes
}

type serverAPIMuxersListRes struct {
	data *defs.APIHLSMuxerList
	err  error
}

type serverAPIMuxersListReq struct {
	res chan serverAPIMuxersListRes
}

type serverAPIMuxersGetRes struct {
	data *defs.APIHLSMuxer
	err  error
}

type serverAPIMuxersGetReq struct {
	name string
	res  chan serverAPIMuxersGetRes
}

type serverAPISessionsListRes struct {
	data *defs.APIHLSSessionList
	err  error
}

type serverAPISessionsListReq struct {
	res chan serverAPISessionsListRes
}

type serverAPISessionsGetRes struct {
	data *defs.APIHLSSession
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

type serverMetrics interface {
	SetHLSServer(defs.APIHLSServer)
}

type serverPathManager interface {
	SetHLSServer(*Server) []defs.Path
	FindPathConf(req defs.PathFindPathConfReq) (*defs.PathFindPathConfRes, error)
	AddReader(req defs.PathAddReaderReq) (*defs.PathAddReaderRes, error)
}

type serverParent interface {
	logger.Writer
}

// Server is a HLS server.
type Server struct {
	Address         string
	DumpPackets     bool
	Encryption      bool
	ServerKey       string
	ServerCert      string
	AllowOrigins    []string
	TrustedProxies  conf.IPNetworks
	AlwaysRemux     bool
	Variant         conf.HLSVariant
	SegmentCount    int
	SegmentDuration conf.Duration
	PartDuration    conf.Duration
	SegmentMaxSize  conf.StringSize
	Directory       string
	ReadTimeout     conf.Duration
	WriteTimeout    conf.Duration
	MuxerCloseAfter conf.Duration
	ExternalCmdPool *externalcmd.Pool
	Metrics         serverMetrics
	PathManager     serverPathManager
	Parent          serverParent

	ctx        context.Context
	ctxCancel  func()
	wg         sync.WaitGroup
	httpServer *httpServer
	muxers     map[string]*muxer

	// in
	chPathReady       chan defs.Path
	chPathNotReady    chan defs.Path
	chGetMuxer        chan serverGetMuxerReq
	chCloseMuxer      chan *muxer
	chAPIMuxerList    chan serverAPIMuxersListReq
	chAPIMuxerGet     chan serverAPIMuxersGetReq
	chAPISessionsList chan serverAPISessionsListReq
	chAPISessionsGet  chan serverAPISessionsGetReq
	chAPISessionsKick chan serverAPISessionsKickReq
}

// Initialize initializes the server.
func (s *Server) Initialize() error {
	ctx, ctxCancel := context.WithCancel(context.Background())

	s.ctx = ctx
	s.ctxCancel = ctxCancel
	s.muxers = make(map[string]*muxer)
	s.chPathReady = make(chan defs.Path)
	s.chPathNotReady = make(chan defs.Path)
	s.chGetMuxer = make(chan serverGetMuxerReq)
	s.chCloseMuxer = make(chan *muxer)
	s.chAPIMuxerList = make(chan serverAPIMuxersListReq)
	s.chAPIMuxerGet = make(chan serverAPIMuxersGetReq)
	s.chAPISessionsList = make(chan serverAPISessionsListReq)
	s.chAPISessionsGet = make(chan serverAPISessionsGetReq)
	s.chAPISessionsKick = make(chan serverAPISessionsKickReq)

	s.httpServer = &httpServer{
		address:        s.Address,
		dumpPackets:    s.DumpPackets,
		encryption:     s.Encryption,
		serverKey:      s.ServerKey,
		serverCert:     s.ServerCert,
		allowOrigins:   s.AllowOrigins,
		trustedProxies: s.TrustedProxies,
		readTimeout:    s.ReadTimeout,
		writeTimeout:   s.WriteTimeout,
		pathManager:    s.PathManager,
		parent:         s,
	}
	err := s.httpServer.initialize()
	if err != nil {
		ctxCancel()
		return err
	}

	str := "listener opened on " + s.Address
	if !s.Encryption {
		str += " (TCP/HTTP)"
	} else {
		str += " (TCP/HTTPS)"
	}
	s.Log(logger.Info, str)

	s.wg.Add(1)
	go s.run()

	if !interfaceIsEmpty(s.Metrics) {
		s.Metrics.SetHLSServer(s)
	}

	return nil
}

// Log implements logger.Writer.
func (s *Server) Log(level logger.Level, format string, args ...any) {
	s.Parent.Log(level, "[HLS] "+format, args...)
}

// Close closes the server.
func (s *Server) Close() {
	s.Log(logger.Info, "listener is closing")

	if !interfaceIsEmpty(s.Metrics) {
		s.Metrics.SetHLSServer(nil)
	}

	s.ctxCancel()
	s.wg.Wait()
}

func (s *Server) run() {
	defer s.wg.Done()

	readyPaths := s.PathManager.SetHLSServer(s)
	defer s.PathManager.SetHLSServer(nil)

	if s.AlwaysRemux {
		for _, pa := range readyPaths {
			if !pa.SafeConf().SourceOnDemand {
				if _, ok := s.muxers[pa.Name()]; !ok {
					s.createMuxer(pa.Name(), "", "")
				}
			}
		}
	}

outer:
	for {
		select {
		case pa := <-s.chPathReady:
			if s.AlwaysRemux && !pa.SafeConf().SourceOnDemand {
				if _, ok := s.muxers[pa.Name()]; !ok {
					s.createMuxer(pa.Name(), "", "")
				}
			}

		case pa := <-s.chPathNotReady:
			c, ok := s.muxers[pa.Name()]
			if ok && c.remoteAddr == "" { // created with "always remux"
				c.Close()
				delete(s.muxers, pa.Name())
			}

		case req := <-s.chGetMuxer:
			mux, ok := s.muxers[req.path]
			switch {
			case ok:
				req.res <- serverGetMuxerRes{muxer: mux}
			case !req.create:
				req.res <- serverGetMuxerRes{err: fmt.Errorf("muxer not found")}
			case s.AlwaysRemux && !req.sourceOnDemand:
				req.res <- serverGetMuxerRes{err: fmt.Errorf("muxer is waiting to be created")}
			default:
				req.res <- serverGetMuxerRes{muxer: s.createMuxer(req.path, req.remoteAddr, req.query)}
			}

		case c := <-s.chCloseMuxer:
			if c2, ok := s.muxers[c.PathName()]; ok && c2 == c {
				delete(s.muxers, c.PathName())
			}

		case req := <-s.chAPIMuxerList:
			data := &defs.APIHLSMuxerList{
				Items: []defs.APIHLSMuxer{},
			}

			for _, muxer := range s.muxers {
				data.Items = append(data.Items, *muxer.apiItem())
			}

			sort.Slice(data.Items, func(i, j int) bool {
				return data.Items[i].Created.Before(data.Items[j].Created)
			})

			req.res <- serverAPIMuxersListRes{
				data: data,
			}

		case req := <-s.chAPIMuxerGet:
			muxer, ok := s.muxers[req.name]
			if !ok {
				req.res <- serverAPIMuxersGetRes{err: ErrMuxerNotFound}
				continue
			}

			req.res <- serverAPIMuxersGetRes{data: muxer.apiItem()}

		case req := <-s.chAPISessionsList:
			data := &defs.APIHLSSessionList{
				Items: []defs.APIHLSSession{},
			}

			for _, muxer := range s.muxers {
				data.Items = append(data.Items, muxer.apiSessionsList()...)
			}

			sort.Slice(data.Items, func(i, j int) bool {
				return data.Items[i].Created.Before(data.Items[j].Created)
			})

			req.res <- serverAPISessionsListRes{data: data}

		case req := <-s.chAPISessionsGet:
			for _, muxer := range s.muxers {
				session, ok := muxer.apiSessionsGet(req.uuid)
				if ok {
					req.res <- serverAPISessionsGetRes{data: session}
					continue outer
				}
			}

			req.res <- serverAPISessionsGetRes{err: ErrSessionNotFound}

		case req := <-s.chAPISessionsKick:
			for _, muxer := range s.muxers {
				ok := muxer.apiSessionsKick(req.uuid)
				if ok {
					req.res <- serverAPISessionsKickRes{}
					continue outer
				}
			}

			req.res <- serverAPISessionsKickRes{err: ErrSessionNotFound}

		case <-s.ctx.Done():
			break outer
		}
	}

	s.ctxCancel()

	s.httpServer.close()
}

func (s *Server) createMuxer(pathName string, remoteAddr string, query string) *muxer {
	r := &muxer{
		parentCtx:       s.ctx,
		remoteAddr:      remoteAddr,
		variant:         s.Variant,
		segmentCount:    s.SegmentCount,
		segmentDuration: s.SegmentDuration,
		partDuration:    s.PartDuration,
		segmentMaxSize:  s.SegmentMaxSize,
		directory:       s.Directory,
		wg:              &s.wg,
		pathName:        pathName,
		pathManager:     s.PathManager,
		parent:          s,
		query:           query,
		closeAfter:      s.MuxerCloseAfter,
	}
	r.initialize()
	s.muxers[pathName] = r
	return r
}

// closeMuxer is called by muxer.
func (s *Server) closeMuxer(c *muxer) {
	select {
	case s.chCloseMuxer <- c:
	case <-s.ctx.Done():
	}
}

func (s *Server) getMuxer(req serverGetMuxerReq) (*muxer, error) {
	req.res = make(chan serverGetMuxerRes)

	select {
	case s.chGetMuxer <- req:
		res := <-req.res
		return res.muxer, res.err

	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}

// PathReady is called by pathManager.
func (s *Server) PathReady(pa defs.Path) {
	select {
	case s.chPathReady <- pa:
	case <-s.ctx.Done():
	}
}

// PathNotReady is called by pathManager.
func (s *Server) PathNotReady(pa defs.Path) {
	select {
	case s.chPathNotReady <- pa:
	case <-s.ctx.Done():
	}
}

// APIMuxersList implements defs.APIHLSServer.
func (s *Server) APIMuxersList() (*defs.APIHLSMuxerList, error) {
	req := serverAPIMuxersListReq{
		res: make(chan serverAPIMuxersListRes),
	}

	select {
	case s.chAPIMuxerList <- req:
		res := <-req.res
		return res.data, res.err

	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}

// APIMuxersGet implements defs.APIHLSServer.
func (s *Server) APIMuxersGet(name string) (*defs.APIHLSMuxer, error) {
	req := serverAPIMuxersGetReq{
		name: name,
		res:  make(chan serverAPIMuxersGetRes),
	}

	select {
	case s.chAPIMuxerGet <- req:
		res := <-req.res
		return res.data, res.err

	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}

// APISessionsList implements defs.APIHLSServer.
func (s *Server) APISessionsList() (*defs.APIHLSSessionList, error) {
	req := serverAPISessionsListReq{
		res: make(chan serverAPISessionsListRes),
	}

	select {
	case s.chAPISessionsList <- req:
		res := <-req.res
		return res.data, res.err

	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}

// APISessionsGet implements defs.APIHLSServer.
func (s *Server) APISessionsGet(uuid uuid.UUID) (*defs.APIHLSSession, error) {
	req := serverAPISessionsGetReq{
		uuid: uuid,
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

// APISessionsKick implements defs.APIHLSServer.
func (s *Server) APISessionsKick(uuid uuid.UUID) error {
	req := serverAPISessionsKickReq{
		uuid: uuid,
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
