// Package hls contains a HLS server.
package hls

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
)

// ErrMuxerNotFound is returned when a muxer is not found.
var ErrMuxerNotFound = errors.New("muxer not found")

type serverGetMuxerRes struct {
	muxer *muxer
	err   error
}

type serverGetMuxerReq struct {
	path           string
	remoteAddr     string
	query          string
	sourceOnDemand bool
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

type serverPathManager interface {
	FindPathConf(req defs.PathFindPathConfReq) (*conf.Path, error)
	AddReader(req defs.PathAddReaderReq) (defs.Path, *stream.Stream, error)
}

type serverParent interface {
	logger.Writer
}

// Server is a HLS server.
type Server struct {
	Address         string
	Encryption      bool
	ServerKey       string
	ServerCert      string
	AllowOrigin     string
	TrustedProxies  conf.IPNetworks
	AlwaysRemux     bool
	Variant         conf.HLSVariant
	SegmentCount    int
	SegmentDuration conf.StringDuration
	PartDuration    conf.StringDuration
	SegmentMaxSize  conf.StringSize
	Directory       string
	ReadTimeout     conf.StringDuration
	MuxerCloseAfter conf.StringDuration
	PathManager     serverPathManager
	Parent          serverParent

	ctx        context.Context
	ctxCancel  func()
	wg         sync.WaitGroup
	httpServer *httpServer
	muxers     map[string]*muxer

	// in
	chPathReady    chan defs.Path
	chPathNotReady chan defs.Path
	chGetMuxer     chan serverGetMuxerReq
	chCloseMuxer   chan *muxer
	chAPIMuxerList chan serverAPIMuxersListReq
	chAPIMuxerGet  chan serverAPIMuxersGetReq
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

	s.httpServer = &httpServer{
		address:        s.Address,
		encryption:     s.Encryption,
		serverKey:      s.ServerKey,
		serverCert:     s.ServerCert,
		allowOrigin:    s.AllowOrigin,
		trustedProxies: s.TrustedProxies,
		readTimeout:    s.ReadTimeout,
		pathManager:    s.PathManager,
		parent:         s,
	}
	err := s.httpServer.initialize()
	if err != nil {
		ctxCancel()
		return err
	}

	s.Log(logger.Info, "listener opened on "+s.Address)

	s.wg.Add(1)
	go s.run()

	return nil
}

// Log implements logger.Writer.
func (s *Server) Log(level logger.Level, format string, args ...interface{}) {
	s.Parent.Log(level, "[HLS] "+format, args...)
}

// Close closes the server.
func (s *Server) Close() {
	s.Log(logger.Info, "listener is closing")
	s.ctxCancel()
	s.wg.Wait()
}

func (s *Server) run() {
	defer s.wg.Done()

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
				Items: []*defs.APIHLSMuxer{},
			}

			for _, muxer := range s.muxers {
				data.Items = append(data.Items, muxer.apiItem())
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

// APIMuxersList is called by api.
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

// APIMuxersGet is called by api.
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
