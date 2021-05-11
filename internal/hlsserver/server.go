package hlsserver

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aler9/rtsp-simple-server/internal/hlsconverter"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/pathman"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

// Parent is implemented by program.
type Parent interface {
	Log(logger.Level, string, ...interface{})
}

// Server is an HLS server.
type Server struct {
	hlsSegmentCount    int
	hlsSegmentDuration time.Duration
	readBufferCount    int
	stats              *stats.Stats
	pathMan            *pathman.PathManager
	parent             Parent

	ctx        context.Context
	ctxCancel  func()
	wg         sync.WaitGroup
	ln         net.Listener
	converters map[string]*hlsconverter.Converter

	// in
	request   chan hlsconverter.Request
	connClose chan *hlsconverter.Converter
}

// New allocates a Server.
func New(
	ctxParent context.Context,
	address string,
	hlsSegmentCount int,
	hlsSegmentDuration time.Duration,
	readBufferCount int,
	stats *stats.Stats,
	pathMan *pathman.PathManager,
	parent Parent,
) (*Server, error) {

	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	ctx, ctxCancel := context.WithCancel(ctxParent)

	s := &Server{
		hlsSegmentCount:    hlsSegmentCount,
		hlsSegmentDuration: hlsSegmentDuration,
		readBufferCount:    readBufferCount,
		stats:              stats,
		pathMan:            pathMan,
		parent:             parent,
		ctx:                ctx,
		ctxCancel:          ctxCancel,
		ln:                 ln,
		converters:         make(map[string]*hlsconverter.Converter),
		request:            make(chan hlsconverter.Request),
		connClose:          make(chan *hlsconverter.Converter),
	}

	s.Log(logger.Info, "listener opened on "+address)

	s.wg.Add(1)
	go s.run()

	return s, nil
}

// Log is the main logging function.
func (s *Server) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[HLS] "+format, append([]interface{}{}, args...)...)
}

// Close closes all the server resources.
func (s *Server) Close() {
	s.ctxCancel()
	s.wg.Wait()
}

func (s *Server) run() {
	defer s.wg.Done()

	hs := &http.Server{Handler: s}
	go hs.Serve(s.ln)

outer:
	for {
		select {
		case req := <-s.request:
			c, ok := s.converters[req.Path]
			if !ok {
				c = hlsconverter.New(
					s.ctx,
					s.hlsSegmentCount,
					s.hlsSegmentDuration,
					s.readBufferCount,
					&s.wg,
					s.stats,
					req.Path,
					s.pathMan,
					s)
				s.converters[req.Path] = c
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
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Log(logger.Info, "[conn %v] %s %s", r.RemoteAddr, r.Method, r.URL.Path)

	// remove leading prefix
	path := r.URL.Path[1:]

	if path == "" || path == "favicon.ico" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		w.Header().Add("Location", parts[0]+"/")
		w.WriteHeader(http.StatusMovedPermanently)
		return
	}

	cres := make(chan io.Reader)
	hreq := hlsconverter.Request{
		Path:    parts[0],
		Subpath: parts[1],
		Req:     r,
		W:       w,
		Res:     cres,
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

func (s *Server) doConverterClose(c *hlsconverter.Converter) {
	delete(s.converters, c.PathName())
	c.ParentClose()
}

// OnConverterClose is called by hlsconverter.Converter.
func (s *Server) OnConverterClose(c *hlsconverter.Converter) {
	select {
	case s.connClose <- c:
	case <-s.ctx.Done():
	}
}
