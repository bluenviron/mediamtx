package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aler9/mediamtx/internal/conf"
	"github.com/aler9/mediamtx/internal/logger"
)

type nilWriter struct{}

func (nilWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

type hlsManagerAPIMuxersListItem struct {
	Created     time.Time `json:"created"`
	LastRequest time.Time `json:"lastRequest"`
	BytesSent   uint64    `json:"bytesSent"`
}

type hlsManagerAPIMuxersListData struct {
	Items map[string]hlsManagerAPIMuxersListItem `json:"items"`
}

type hlsManagerAPIMuxersListRes struct {
	data   *hlsManagerAPIMuxersListData
	muxers map[string]*hlsMuxer
	err    error
}

type hlsManagerAPIMuxersListReq struct {
	res chan hlsManagerAPIMuxersListRes
}

type hlsManagerAPIMuxersListSubReq struct {
	data *hlsManagerAPIMuxersListData
	res  chan struct{}
}

type hlsManagerParent interface {
	logger.Writer
}

type hlsManager struct {
	externalAuthenticationURL string
	alwaysRemux               bool
	variant                   conf.HLSVariant
	segmentCount              int
	segmentDuration           conf.StringDuration
	partDuration              conf.StringDuration
	segmentMaxSize            conf.StringSize
	directory                 string
	readBufferCount           int
	pathManager               *pathManager
	metrics                   *metrics
	parent                    hlsManagerParent

	ctx        context.Context
	ctxCancel  func()
	wg         sync.WaitGroup
	httpServer *hlsHTTPServer
	muxers     map[string]*hlsMuxer

	// in
	chPathSourceReady    chan *path
	chPathSourceNotReady chan *path
	chHandleRequest      chan hlsMuxerHandleRequestReq
	chMuxerClose         chan *hlsMuxer
	chAPIMuxerList       chan hlsManagerAPIMuxersListReq
}

func newHLSManager(
	parentCtx context.Context,
	address string,
	encryption bool,
	serverKey string,
	serverCert string,
	externalAuthenticationURL string,
	alwaysRemux bool,
	variant conf.HLSVariant,
	segmentCount int,
	segmentDuration conf.StringDuration,
	partDuration conf.StringDuration,
	segmentMaxSize conf.StringSize,
	allowOrigin string,
	trustedProxies conf.IPsOrCIDRs,
	directory string,
	readTimeout conf.StringDuration,
	readBufferCount int,
	pathManager *pathManager,
	metrics *metrics,
	parent hlsManagerParent,
) (*hlsManager, error) {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	s := &hlsManager{
		externalAuthenticationURL: externalAuthenticationURL,
		alwaysRemux:               alwaysRemux,
		variant:                   variant,
		segmentCount:              segmentCount,
		segmentDuration:           segmentDuration,
		partDuration:              partDuration,
		segmentMaxSize:            segmentMaxSize,
		directory:                 directory,
		readBufferCount:           readBufferCount,
		pathManager:               pathManager,
		parent:                    parent,
		metrics:                   metrics,
		ctx:                       ctx,
		ctxCancel:                 ctxCancel,
		muxers:                    make(map[string]*hlsMuxer),
		chPathSourceReady:         make(chan *path),
		chPathSourceNotReady:      make(chan *path),
		chHandleRequest:           make(chan hlsMuxerHandleRequestReq),
		chMuxerClose:              make(chan *hlsMuxer),
		chAPIMuxerList:            make(chan hlsManagerAPIMuxersListReq),
	}

	var err error
	s.httpServer, err = newHLSHTTPServer(
		address,
		encryption,
		serverKey,
		serverCert,
		allowOrigin,
		trustedProxies,
		readTimeout,
		s,
	)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	s.Log(logger.Info, "listener opened on "+address)

	s.pathManager.hlsManagerSet(s)

	if s.metrics != nil {
		s.metrics.hlsManagerSet(s)
	}

	s.wg.Add(1)
	go s.run()

	return s, nil
}

// Log is the main logging function.
func (s *hlsManager) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[HLS] "+format, append([]interface{}{}, args...)...)
}

func (s *hlsManager) close() {
	s.Log(logger.Info, "listener is closing")
	s.ctxCancel()
	s.wg.Wait()
}

func (s *hlsManager) run() {
	defer s.wg.Done()

outer:
	for {
		select {
		case pa := <-s.chPathSourceReady:
			if s.alwaysRemux {
				s.createMuxer(pa.name, "")
			}

		case pa := <-s.chPathSourceNotReady:
			if s.alwaysRemux {
				c, ok := s.muxers[pa.name]
				if ok {
					c.close()
					delete(s.muxers, pa.name)
				}
			}

		case req := <-s.chHandleRequest:
			r, ok := s.muxers[req.path]
			switch {
			case ok:
				r.processRequest(&req)

			case s.alwaysRemux:
				req.res <- nil

			default:
				r := s.createMuxer(req.path, req.ctx.ClientIP())
				r.processRequest(&req)
			}

		case c := <-s.chMuxerClose:
			if c2, ok := s.muxers[c.PathName()]; !ok || c2 != c {
				continue
			}
			delete(s.muxers, c.PathName())

		case req := <-s.chAPIMuxerList:
			muxers := make(map[string]*hlsMuxer)

			for name, m := range s.muxers {
				muxers[name] = m
			}

			req.res <- hlsManagerAPIMuxersListRes{
				muxers: muxers,
			}

		case <-s.ctx.Done():
			break outer
		}
	}

	s.ctxCancel()

	s.httpServer.close()

	s.pathManager.hlsManagerSet(nil)

	if s.metrics != nil {
		s.metrics.hlsManagerSet(nil)
	}
}

func (s *hlsManager) createMuxer(pathName string, remoteAddr string) *hlsMuxer {
	r := newHLSMuxer(
		s.ctx,
		remoteAddr,
		s.externalAuthenticationURL,
		s.alwaysRemux,
		s.variant,
		s.segmentCount,
		s.segmentDuration,
		s.partDuration,
		s.segmentMaxSize,
		s.directory,
		s.readBufferCount,
		&s.wg,
		pathName,
		s.pathManager,
		s)
	s.muxers[pathName] = r
	return r
}

// muxerClose is called by hlsMuxer.
func (s *hlsManager) muxerClose(c *hlsMuxer) {
	select {
	case s.chMuxerClose <- c:
	case <-s.ctx.Done():
	}
}

// pathSourceReady is called by pathManager.
func (s *hlsManager) pathSourceReady(pa *path) {
	select {
	case s.chPathSourceReady <- pa:
	case <-s.ctx.Done():
	}
}

// pathSourceNotReady is called by pathManager.
func (s *hlsManager) pathSourceNotReady(pa *path) {
	select {
	case s.chPathSourceNotReady <- pa:
	case <-s.ctx.Done():
	}
}

// apiMuxersList is called by api.
func (s *hlsManager) apiMuxersList() hlsManagerAPIMuxersListRes {
	req := hlsManagerAPIMuxersListReq{
		res: make(chan hlsManagerAPIMuxersListRes),
	}

	select {
	case s.chAPIMuxerList <- req:
		res := <-req.res

		res.data = &hlsManagerAPIMuxersListData{
			Items: make(map[string]hlsManagerAPIMuxersListItem),
		}

		for _, pa := range res.muxers {
			pa.apiMuxersList(hlsManagerAPIMuxersListSubReq{data: res.data})
		}

		return res

	case <-s.ctx.Done():
		return hlsManagerAPIMuxersListRes{err: fmt.Errorf("terminated")}
	}
}

func (s *hlsManager) handleRequest(req hlsMuxerHandleRequestReq) {
	req.res = make(chan *hlsMuxer)

	select {
	case s.chHandleRequest <- req:
		muxer := <-req.res
		if muxer != nil {
			req.ctx.Request.URL.Path = req.file
			muxer.handleRequest(req.ctx)
		}

	case <-s.ctx.Done():
	}
}
