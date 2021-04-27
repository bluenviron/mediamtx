package serverhls

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aler9/rtsp-simple-server/internal/converterhls"
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

	ln         net.Listener
	wg         sync.WaitGroup
	converters map[string]*converterhls.Converter

	// in
	request     chan converterhls.Request
	clientClose chan *converterhls.Converter
	terminate   chan struct{}

	// out
	done chan struct{}
}

// New allocates a Server.
func New(
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

	s := &Server{
		hlsSegmentCount:    hlsSegmentCount,
		hlsSegmentDuration: hlsSegmentDuration,
		readBufferCount:    readBufferCount,
		stats:              stats,
		pathMan:            pathMan,
		parent:             parent,
		ln:                 ln,
		converters:         make(map[string]*converterhls.Converter),
		request:            make(chan converterhls.Request),
		clientClose:        make(chan *converterhls.Converter),
		terminate:          make(chan struct{}),
		done:               make(chan struct{}),
	}

	s.Log(logger.Info, "listener opened on "+address)

	go s.run()

	return s, nil
}

// Log is the main logging function.
func (s *Server) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[HLS] "+format, append([]interface{}{}, args...)...)
}

// Close closes all the server resources.
func (s *Server) Close() {
	close(s.terminate)
	<-s.done
}

func (s *Server) run() {
	defer close(s.done)

	hs := &http.Server{Handler: s}
	go hs.Serve(s.ln)

outer:
	for {
		select {
		case req := <-s.request:
			c, ok := s.converters[req.Path]
			if !ok {
				c = converterhls.New(
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

		case c := <-s.clientClose:
			if c2, ok := s.converters[c.PathName()]; !ok || c2 != c {
				continue
			}
			s.doConverterClose(c)

		case <-s.terminate:
			break outer
		}
	}

	go func() {
		for {
			select {
			case req, ok := <-s.request:
				if !ok {
					return
				}
				req.Res <- nil

			case _, ok := <-s.clientClose:
				if !ok {
					return
				}
			}
		}
	}()

	for _, c := range s.converters {
		s.doConverterClose(c)
	}

	hs.Shutdown(context.Background())

	close(s.request)
	close(s.clientClose)
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Log(logger.Info, "[client %s] %s %s", r.RemoteAddr, r.Method, r.URL.Path)

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
	s.request <- converterhls.Request{
		Path:    parts[0],
		Subpath: parts[1],
		Req:     r,
		W:       w,
		Res:     cres,
	}
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
}

func (s *Server) doConverterClose(c *converterhls.Converter) {
	delete(s.converters, c.PathName())
	c.Close()
}

// OnConverterClose is called by converterhls.Converter.
func (s *Server) OnConverterClose(c *converterhls.Converter) {
	s.clientClose <- c
}
