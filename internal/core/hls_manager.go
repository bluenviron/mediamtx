package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
)

type hlsManagerAPIMuxersListItem struct {
	Path        string    `json:"path"`
	Created     time.Time `json:"created"`
	LastRequest time.Time `json:"lastRequest"`
	BytesSent   uint64    `json:"bytesSent"`
}

type hlsManagerAPIMuxersListData struct {
	PageCount int                           `json:"pageCount"`
	Items     []hlsManagerAPIMuxersListItem `json:"items"`
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

	m := &hlsManager{
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
	m.httpServer, err = newHLSHTTPServer(
		address,
		encryption,
		serverKey,
		serverCert,
		allowOrigin,
		trustedProxies,
		readTimeout,
		m.pathManager,
		m,
	)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	m.Log(logger.Info, "listener opened on "+address)

	m.pathManager.hlsManagerSet(m)

	if m.metrics != nil {
		m.metrics.hlsManagerSet(m)
	}

	m.wg.Add(1)
	go m.run()

	return m, nil
}

// Log is the main logging function.
func (m *hlsManager) Log(level logger.Level, format string, args ...interface{}) {
	m.parent.Log(level, "[HLS] "+format, append([]interface{}{}, args...)...)
}

func (m *hlsManager) close() {
	m.Log(logger.Info, "listener is closing")
	m.ctxCancel()
	m.wg.Wait()
}

func (m *hlsManager) run() {
	defer m.wg.Done()

outer:
	for {
		select {
		case pa := <-m.chPathSourceReady:
			if m.alwaysRemux {
				m.createMuxer(pa.name, "")
			}

		case pa := <-m.chPathSourceNotReady:
			if m.alwaysRemux {
				c, ok := m.muxers[pa.name]
				if ok {
					c.close()
					delete(m.muxers, pa.name)
				}
			}

		case req := <-m.chHandleRequest:
			r, ok := m.muxers[req.path]
			switch {
			case ok:
				r.processRequest(&req)

			case m.alwaysRemux:
				req.res <- nil

			default:
				r := m.createMuxer(req.path, req.ctx.ClientIP())
				r.processRequest(&req)
			}

		case c := <-m.chMuxerClose:
			if c2, ok := m.muxers[c.PathName()]; !ok || c2 != c {
				continue
			}
			delete(m.muxers, c.PathName())

		case req := <-m.chAPIMuxerList:
			muxers := make(map[string]*hlsMuxer)

			for name, m := range m.muxers {
				muxers[name] = m
			}

			req.res <- hlsManagerAPIMuxersListRes{
				muxers: muxers,
			}

		case <-m.ctx.Done():
			break outer
		}
	}

	m.ctxCancel()

	m.httpServer.close()

	m.pathManager.hlsManagerSet(nil)

	if m.metrics != nil {
		m.metrics.hlsManagerSet(nil)
	}
}

func (m *hlsManager) createMuxer(pathName string, remoteAddr string) *hlsMuxer {
	r := newHLSMuxer(
		m.ctx,
		remoteAddr,
		m.externalAuthenticationURL,
		m.alwaysRemux,
		m.variant,
		m.segmentCount,
		m.segmentDuration,
		m.partDuration,
		m.segmentMaxSize,
		m.directory,
		m.readBufferCount,
		&m.wg,
		pathName,
		m.pathManager,
		m)
	m.muxers[pathName] = r
	return r
}

// muxerClose is called by hlsMuxer.
func (m *hlsManager) muxerClose(c *hlsMuxer) {
	select {
	case m.chMuxerClose <- c:
	case <-m.ctx.Done():
	}
}

// pathSourceReady is called by pathManager.
func (m *hlsManager) pathSourceReady(pa *path) {
	select {
	case m.chPathSourceReady <- pa:
	case <-m.ctx.Done():
	}
}

// pathSourceNotReady is called by pathManager.
func (m *hlsManager) pathSourceNotReady(pa *path) {
	select {
	case m.chPathSourceNotReady <- pa:
	case <-m.ctx.Done():
	}
}

// apiMuxersList is called by api.
func (m *hlsManager) apiMuxersList() hlsManagerAPIMuxersListRes {
	req := hlsManagerAPIMuxersListReq{
		res: make(chan hlsManagerAPIMuxersListRes),
	}

	select {
	case m.chAPIMuxerList <- req:
		res := <-req.res

		res.data = &hlsManagerAPIMuxersListData{
			Items: []hlsManagerAPIMuxersListItem{},
		}

		for _, pa := range res.muxers {
			pa.apiMuxersList(hlsManagerAPIMuxersListSubReq{data: res.data})
		}

		return res

	case <-m.ctx.Done():
		return hlsManagerAPIMuxersListRes{err: fmt.Errorf("terminated")}
	}
}

func (m *hlsManager) handleRequest(req hlsMuxerHandleRequestReq) {
	req.res = make(chan *hlsMuxer)

	select {
	case m.chHandleRequest <- req:
		muxer := <-req.res
		if muxer != nil {
			req.ctx.Request.URL.Path = req.file
			muxer.handleRequest(req.ctx)
		}

	case <-m.ctx.Done():
	}
}
