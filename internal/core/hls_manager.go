package core

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
)

type hlsManagerAPIMuxersListRes struct {
	data *apiHLSMuxersList
	err  error
}

type hlsManagerAPIMuxersListReq struct {
	res chan hlsManagerAPIMuxersListRes
}

type hlsManagerAPIMuxersGetRes struct {
	data *apiHLSMuxer
	err  error
}

type hlsManagerAPIMuxersGetReq struct {
	name string
	res  chan hlsManagerAPIMuxersGetRes
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
	chPathReady     chan *path
	chPathNotReady  chan *path
	chHandleRequest chan hlsMuxerHandleRequestReq
	chCloseMuxer    chan *hlsMuxer
	chAPIMuxerList  chan hlsManagerAPIMuxersListReq
	chAPIMuxerGet   chan hlsManagerAPIMuxersGetReq
}

func newHLSManager(
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
	ctx, ctxCancel := context.WithCancel(context.Background())

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
		chPathReady:               make(chan *path),
		chPathNotReady:            make(chan *path),
		chHandleRequest:           make(chan hlsMuxerHandleRequestReq),
		chCloseMuxer:              make(chan *hlsMuxer),
		chAPIMuxerList:            make(chan hlsManagerAPIMuxersListReq),
		chAPIMuxerGet:             make(chan hlsManagerAPIMuxersGetReq),
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

	m.pathManager.setHLSManager(m)

	if m.metrics != nil {
		m.metrics.setHLSManager(m)
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
		case pa := <-m.chPathReady:
			if m.alwaysRemux && !pa.conf.SourceOnDemand {
				if _, ok := m.muxers[pa.name]; !ok {
					m.createMuxer(pa.name, "")
				}
			}

		case pa := <-m.chPathNotReady:
			c, ok := m.muxers[pa.name]
			if ok && c.remoteAddr == "" { // created with "always remux"
				c.close()
				delete(m.muxers, pa.name)
			}

		case req := <-m.chHandleRequest:
			r, ok := m.muxers[req.path]
			switch {
			case ok:
				r.processRequest(&req)

			default:
				r := m.createMuxer(req.path, req.ctx.ClientIP())
				r.processRequest(&req)
			}

		case c := <-m.chCloseMuxer:
			if c2, ok := m.muxers[c.PathName()]; !ok || c2 != c {
				continue
			}
			delete(m.muxers, c.PathName())

		case req := <-m.chAPIMuxerList:
			data := &apiHLSMuxersList{
				Items: []*apiHLSMuxer{},
			}

			for _, muxer := range m.muxers {
				data.Items = append(data.Items, muxer.apiItem())
			}

			sort.Slice(data.Items, func(i, j int) bool {
				return data.Items[i].Created.Before(data.Items[j].Created)
			})

			req.res <- hlsManagerAPIMuxersListRes{
				data: data,
			}

		case req := <-m.chAPIMuxerGet:
			muxer, ok := m.muxers[req.name]
			if !ok {
				req.res <- hlsManagerAPIMuxersGetRes{err: errAPINotFound}
				continue
			}

			req.res <- hlsManagerAPIMuxersGetRes{data: muxer.apiItem()}

		case <-m.ctx.Done():
			break outer
		}
	}

	m.ctxCancel()

	m.httpServer.close()

	m.pathManager.setHLSManager(nil)

	if m.metrics != nil {
		m.metrics.setHLSManager(nil)
	}
}

func (m *hlsManager) createMuxer(pathName string, remoteAddr string) *hlsMuxer {
	r := newHLSMuxer(
		m.ctx,
		remoteAddr,
		m.externalAuthenticationURL,
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

// closeMuxer is called by hlsMuxer.
func (m *hlsManager) closeMuxer(c *hlsMuxer) {
	select {
	case m.chCloseMuxer <- c:
	case <-m.ctx.Done():
	}
}

// pathReady is called by pathManager.
func (m *hlsManager) pathReady(pa *path) {
	select {
	case m.chPathReady <- pa:
	case <-m.ctx.Done():
	}
}

// pathNotReady is called by pathManager.
func (m *hlsManager) pathNotReady(pa *path) {
	select {
	case m.chPathNotReady <- pa:
	case <-m.ctx.Done():
	}
}

// apiMuxersList is called by api.
func (m *hlsManager) apiMuxersList() (*apiHLSMuxersList, error) {
	req := hlsManagerAPIMuxersListReq{
		res: make(chan hlsManagerAPIMuxersListRes),
	}

	select {
	case m.chAPIMuxerList <- req:
		res := <-req.res
		return res.data, res.err

	case <-m.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}

// apiMuxersGet is called by api.
func (m *hlsManager) apiMuxersGet(name string) (*apiHLSMuxer, error) {
	req := hlsManagerAPIMuxersGetReq{
		name: name,
		res:  make(chan hlsManagerAPIMuxersGetRes),
	}

	select {
	case m.chAPIMuxerGet <- req:
		res := <-req.res
		return res.data, res.err

	case <-m.ctx.Done():
		return nil, fmt.Errorf("terminated")
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
