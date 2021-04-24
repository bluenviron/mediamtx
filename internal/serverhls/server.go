package serverhls

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/aler9/rtsp-simple-server/internal/logger"
)

// Request is an HTTP request received by the HLS server.
type Request struct {
	Path    string
	Subpath string
	Req     *http.Request
	W       http.ResponseWriter
	Res     chan io.Reader
}

// Parent is implemented by program.
type Parent interface {
	Log(logger.Level, string, ...interface{})
}

// Server is an HLS server.
type Server struct {
	parent Parent

	ln net.Listener
	s  *http.Server

	// out
	request chan Request
}

// New allocates a Server.
func New(
	address string,
	parent Parent,
) (*Server, error) {

	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	s := &Server{
		parent:  parent,
		ln:      ln,
		request: make(chan Request),
	}

	s.s = &http.Server{
		Handler: s,
	}

	s.log(logger.Info, "opened on "+address)

	go s.s.Serve(s.ln)

	return s, nil
}

func (s *Server) log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[HLS listener] "+format, append([]interface{}{}, args...)...)
}

// Close closes all the server resources.
func (s *Server) Close() {
	go func() {
		for req := range s.request {
			req.Res <- nil
		}
	}()
	s.s.Shutdown(context.Background())
	close(s.request)
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.log(logger.Info, "%s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

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
	s.request <- Request{
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

// Request returns a channel to handle incoming HTTP requests.
func (s *Server) Request() chan Request {
	return s.request
}
