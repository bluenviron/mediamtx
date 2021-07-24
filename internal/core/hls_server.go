package core

import (
	"context"
	"io"
	"net"
	"net/http"
	gopath "path"
	"strings"
	"sync"
	"time"

	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type hlsServerParent interface {
	Log(logger.Level, string, ...interface{})
}

type hlsServer struct {
	hlsSegmentCount    int
	hlsSegmentDuration time.Duration
	hlsAllowOrigin     string
	readBufferCount    int
	stats              *stats
	pathMan            *pathManager
	parent             hlsServerParent

	ctx        context.Context
	ctxCancel  func()
	wg         sync.WaitGroup
	ln         net.Listener
	converters map[string]*hlsConverter

	// in
	request   chan hlsConverterRequest
	connClose chan *hlsConverter
}

func newHLSServer(
	parentCtx context.Context,
	address string,
	hlsSegmentCount int,
	hlsSegmentDuration time.Duration,
	hlsAllowOrigin string,
	readBufferCount int,
	stats *stats,
	pathMan *pathManager,
	parent hlsServerParent,
) (*hlsServer, error) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	ctx, ctxCancel := context.WithCancel(parentCtx)

	s := &hlsServer{
		hlsSegmentCount:    hlsSegmentCount,
		hlsSegmentDuration: hlsSegmentDuration,
		hlsAllowOrigin:     hlsAllowOrigin,
		readBufferCount:    readBufferCount,
		stats:              stats,
		pathMan:            pathMan,
		parent:             parent,
		ctx:                ctx,
		ctxCancel:          ctxCancel,
		ln:                 ln,
		converters:         make(map[string]*hlsConverter),
		request:            make(chan hlsConverterRequest),
		connClose:          make(chan *hlsConverter),
	}

	s.Log(logger.Info, "listener opened on "+address)

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
}

func (s *hlsServer) run() {
	defer s.wg.Done()

	hs := &http.Server{Handler: s}
	go hs.Serve(s.ln)

outer:
	for {
		select {
		case req := <-s.request:
			c, ok := s.converters[req.Dir]
			if !ok {
				c = newHLSConverter(
					s.ctx,
					s.hlsSegmentCount,
					s.hlsSegmentDuration,
					s.readBufferCount,
					&s.wg,
					s.stats,
					req.Dir,
					s.pathMan,
					s)
				s.converters[req.Dir] = c
			}
			c.OnRequest(req)

		case c := <-s.connClose:
			if c2, ok := s.converters[c.PathName()]; !ok || c2 != c {
				continue
			}
			s.doConverterClose(c)

		case <-s.ctx.Done():
			break outer
		}
	}

	s.ctxCancel()

	for _, c := range s.converters {
		s.doConverterClose(c)
	}

	hs.Shutdown(context.Background())
}

// ServeHTTP implements http.Handler.
func (s *hlsServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Log(logger.Info, "[conn %v] %s %s", r.RemoteAddr, r.Method, r.URL.Path)

	// remove leading prefix
	pa := r.URL.Path[1:]

	w.Header().Add("Access-Control-Allow-Origin", s.hlsAllowOrigin)
	w.Header().Add("Access-Control-Allow-Credentials", "true")

	if r.Method == "OPTIONS" {
		w.Header().Add("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Add("Access-Control-Allow-Headers", r.Header.Get("Access-Control-Request-Headers"))
		w.WriteHeader(http.StatusOK)
		return
	}

	if pa == "" || pa == "favicon.ico" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	dir, fname := func() (string, string) {
		if strings.HasSuffix(pa, ".ts") || strings.HasSuffix(pa, ".m3u8") {
			return gopath.Dir(pa), gopath.Base(pa)
		}
		return pa, ""
	}()

	if fname == "" && !strings.HasSuffix(dir, "/") {
		w.Header().Add("Location", "/"+dir+"/")
		w.WriteHeader(http.StatusMovedPermanently)
		return
	}

	dir = strings.TrimSuffix(dir, "/")

	cres := make(chan io.Reader)
	hreq := hlsConverterRequest{
		Dir:  dir,
		File: fname,
		Req:  r,
		W:    w,
		Res:  cres,
	}

	select {
	case s.request <- hreq:
		res := <-cres

		if res != nil {
			buf := make([]byte, 4096)
			for {
				n, err := res.Read(buf)
				if err != nil {
					return
				}

				_, err = w.Write(buf[:n])
				if err != nil {
					return
				}

				w.(http.Flusher).Flush()
			}
		}

	case <-s.ctx.Done():
	}
}

func (s *hlsServer) doConverterClose(c *hlsConverter) {
	delete(s.converters, c.PathName())
	c.ParentClose()
}

func (s *hlsServer) OnConverterClose(c *hlsConverter) {
	select {
	case s.connClose <- c:
	case <-s.ctx.Done():
	}
}
