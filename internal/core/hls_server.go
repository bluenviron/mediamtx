package core

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	gopath "path"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type hlsServerParent interface {
	Log(logger.Level, string, ...interface{})
}

type hlsServer struct {
	hlsAlwaysRemux     bool
	hlsSegmentCount    int
	hlsSegmentDuration conf.StringDuration
	hlsAllowOrigin     string
	readBufferCount    int
	pathManager        *pathManager
	parent             hlsServerParent

	ctx       context.Context
	ctxCancel func()
	wg        sync.WaitGroup
	ln        net.Listener
	muxers    map[string]*hlsMuxer

	// in
	pathSourceReady chan *path
	request         chan hlsMuxerRequest
	muxerClose      chan *hlsMuxer
}

func newHLSServer(
	parentCtx context.Context,
	address string,
	hlsAlwaysRemux bool,
	hlsSegmentCount int,
	hlsSegmentDuration conf.StringDuration,
	hlsAllowOrigin string,
	readBufferCount int,
	pathManager *pathManager,
	parent hlsServerParent,
) (*hlsServer, error) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	ctx, ctxCancel := context.WithCancel(parentCtx)

	s := &hlsServer{
		hlsAlwaysRemux:     hlsAlwaysRemux,
		hlsSegmentCount:    hlsSegmentCount,
		hlsSegmentDuration: hlsSegmentDuration,
		hlsAllowOrigin:     hlsAllowOrigin,
		readBufferCount:    readBufferCount,
		pathManager:        pathManager,
		parent:             parent,
		ctx:                ctx,
		ctxCancel:          ctxCancel,
		ln:                 ln,
		muxers:             make(map[string]*hlsMuxer),
		pathSourceReady:    make(chan *path),
		request:            make(chan hlsMuxerRequest),
		muxerClose:         make(chan *hlsMuxer),
	}

	s.Log(logger.Info, "listener opened on "+address)

	s.pathManager.OnHLSServerSet(s)

	s.wg.Add(1)
	go s.run()

	return s, nil
}

// Log is the main logging function.
func (s *hlsServer) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[HLS] "+format, append([]interface{}{}, args...)...)
}

func (s *hlsServer) close() {
	s.ctxCancel()
	s.wg.Wait()
	s.Log(logger.Info, "closed")
}

func (s *hlsServer) run() {
	defer s.wg.Done()

	router := gin.New()
	router.NoRoute(s.onRequest)

	hs := &http.Server{Handler: router}
	go hs.Serve(s.ln)

outer:
	for {
		select {
		case pa := <-s.pathSourceReady:
			if s.hlsAlwaysRemux {
				s.findOrCreateMuxer(pa.Name())
			}

		case req := <-s.request:
			r := s.findOrCreateMuxer(req.Dir)
			r.OnRequest(req)

		case c := <-s.muxerClose:
			if c2, ok := s.muxers[c.PathName()]; !ok || c2 != c {
				continue
			}
			delete(s.muxers, c.PathName())

		case <-s.ctx.Done():
			break outer
		}
	}

	s.ctxCancel()

	hs.Shutdown(context.Background())

	s.pathManager.OnHLSServerSet(nil)
}

func (s *hlsServer) onRequest(ctx *gin.Context) {
	s.Log(logger.Info, "[conn %v] %s %s", ctx.Request.RemoteAddr, ctx.Request.Method, ctx.Request.URL.Path)

	byts, _ := httputil.DumpRequest(ctx.Request, true)
	s.Log(logger.Debug, "[conn %v] [c->s] %s", ctx.Request.RemoteAddr, string(byts))

	logw := &httpLogWriter{ResponseWriter: ctx.Writer}
	ctx.Writer = logw

	ctx.Writer.Header().Set("Server", "rtsp-simple-server")
	ctx.Writer.Header().Set("Access-Control-Allow-Origin", s.hlsAllowOrigin)
	ctx.Writer.Header().Set("Access-Control-Allow-Credentials", "true")

	switch ctx.Request.Method {
	case http.MethodGet:

	case http.MethodOptions:
		ctx.Writer.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		ctx.Writer.Header().Set("Access-Control-Allow-Headers", ctx.Request.Header.Get("Access-Control-Request-Headers"))
		ctx.Writer.WriteHeader(http.StatusOK)
		return

	default:
		ctx.Writer.WriteHeader(http.StatusNotFound)
		return
	}

	// remove leading prefix
	pa := ctx.Request.URL.Path[1:]

	switch pa {
	case "", "favicon.ico":
		ctx.Writer.WriteHeader(http.StatusNotFound)
		return
	}

	dir, fname := func() (string, string) {
		if strings.HasSuffix(pa, ".ts") || strings.HasSuffix(pa, ".m3u8") {
			return gopath.Dir(pa), gopath.Base(pa)
		}
		return pa, ""
	}()

	if fname == "" && !strings.HasSuffix(dir, "/") {
		ctx.Writer.Header().Set("Location", "/"+dir+"/")
		ctx.Writer.WriteHeader(http.StatusMovedPermanently)
		return
	}

	dir = strings.TrimSuffix(dir, "/")

	cres := make(chan hlsMuxerResponse)
	hreq := hlsMuxerRequest{
		Dir:  dir,
		File: fname,
		Req:  ctx.Request,
		Res:  cres,
	}

	select {
	case s.request <- hreq:
		res := <-cres

		for k, v := range res.Header {
			ctx.Writer.Header().Set(k, v)
		}
		ctx.Writer.WriteHeader(res.Status)

		if res.Body != nil {
			io.Copy(ctx.Writer, res.Body)
		}

	case <-s.ctx.Done():
	}

	s.Log(logger.Debug, "[conn %v] [s->c] %s", ctx.Request.RemoteAddr, logw.dump())
}

func (s *hlsServer) findOrCreateMuxer(pathName string) *hlsMuxer {
	r, ok := s.muxers[pathName]
	if !ok {
		r = newHLSMuxer(
			s.ctx,
			s.hlsAlwaysRemux,
			s.hlsSegmentCount,
			s.hlsSegmentDuration,
			s.readBufferCount,
			&s.wg,
			pathName,
			s.pathManager,
			s)
		s.muxers[pathName] = r
	}
	return r
}

// OnMuxerClose is called by hlsMuxer.
func (s *hlsServer) OnMuxerClose(c *hlsMuxer) {
	select {
	case s.muxerClose <- c:
	case <-s.ctx.Done():
	}
}

// OnPathSourceReady is called by core.
func (s *hlsServer) OnPathSourceReady(pa *path) {
	select {
	case s.pathSourceReady <- pa:
	case <-s.ctx.Done():
	}
}
