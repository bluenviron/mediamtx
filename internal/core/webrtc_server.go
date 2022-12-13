package core

import (
	"context"
	"crypto/tls"
	_ "embed"
	"log"
	"net"
	"net/http"
	gopath "path"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

//go:embed webrtc_index.html
var webrtcIndex []byte

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type webRTCConnNewReq struct {
	pathName string
	wsconn   *websocket.Conn
}

type webRTCServerParent interface {
	Log(logger.Level, string, ...interface{})
}

type webRTCServer struct {
	allowOrigin     string
	trustedProxies  conf.IPsOrCIDRs
	stunServers     []string
	readBufferCount int
	pathManager     *pathManager
	parent          webRTCServerParent

	ctx       context.Context
	ctxCancel func()
	wg        sync.WaitGroup
	ln        net.Listener
	tlsConfig *tls.Config
	conns     map[*webRTCConn]struct{}

	// in
	connNew     chan webRTCConnNewReq
	chConnClose chan *webRTCConn
}

func newWebRTCServer(
	parentCtx context.Context,
	address string,
	serverKey string,
	serverCert string,
	allowOrigin string,
	trustedProxies conf.IPsOrCIDRs,
	stunServers []string,
	readBufferCount int,
	pathManager *pathManager,
	parent webRTCServerParent,
) (*webRTCServer, error) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	crt, err := tls.LoadX509KeyPair(serverCert, serverKey)
	if err != nil {
		ln.Close()
		return nil, err
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{crt},
	}

	ctx, ctxCancel := context.WithCancel(parentCtx)

	s := &webRTCServer{
		allowOrigin:     allowOrigin,
		trustedProxies:  trustedProxies,
		stunServers:     stunServers,
		readBufferCount: readBufferCount,
		pathManager:     pathManager,
		parent:          parent,
		ctx:             ctx,
		ctxCancel:       ctxCancel,
		ln:              ln,
		tlsConfig:       tlsConfig,
		conns:           make(map[*webRTCConn]struct{}),
		connNew:         make(chan webRTCConnNewReq),
		chConnClose:     make(chan *webRTCConn),
	}

	s.log(logger.Info, "listener opened on "+address)

	s.wg.Add(1)
	go s.run()

	return s, nil
}

// Log is the main logging function.
func (s *webRTCServer) log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[WebRTC] "+format, append([]interface{}{}, args...)...)
}

func (s *webRTCServer) close() {
	s.log(logger.Info, "listener is closing")
	s.ctxCancel()
	s.wg.Wait()
}

func (s *webRTCServer) run() {
	defer s.wg.Done()

	router := gin.New()
	router.NoRoute(httpLoggerMiddleware(s), s.onRequest)

	tmp := make([]string, len(s.trustedProxies))
	for i, entry := range s.trustedProxies {
		tmp[i] = entry.String()
	}
	router.SetTrustedProxies(tmp)

	hs := &http.Server{
		Handler:   router,
		TLSConfig: s.tlsConfig,
		ErrorLog:  log.New(&nilWriter{}, "", 0),
	}

	if s.tlsConfig != nil {
		go hs.ServeTLS(s.ln, "", "")
	} else {
		go hs.Serve(s.ln)
	}

outer:
	for {
		select {
		case req := <-s.connNew:
			c := newWebRTCConn(
				s.ctx,
				s.readBufferCount,
				req.pathName,
				req.wsconn,
				s.stunServers,
				&s.wg,
				s.pathManager,
				s,
			)
			s.conns[c] = struct{}{}

		case conn := <-s.chConnClose:
			delete(s.conns, conn)

		case <-s.ctx.Done():
			break outer
		}
	}

	s.ctxCancel()

	hs.Shutdown(context.Background())
	s.ln.Close() // in case Shutdown() is called before Serve()
}

func (s *webRTCServer) onRequest(ctx *gin.Context) {
	ctx.Writer.Header().Set("Access-Control-Allow-Origin", s.allowOrigin)
	ctx.Writer.Header().Set("Access-Control-Allow-Credentials", "true")

	switch ctx.Request.Method {
	case http.MethodGet:

	case http.MethodOptions:
		ctx.Writer.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		ctx.Writer.Header().Set("Access-Control-Allow-Headers", ctx.Request.Header.Get("Access-Control-Request-Headers"))
		ctx.Writer.WriteHeader(http.StatusOK)
		return

	default:
		return
	}

	// remove leading prefix
	pa := ctx.Request.URL.Path[1:]

	switch pa {
	case "", "favicon.ico":
		return
	}

	dir, fname := func() (string, string) {
		if strings.HasSuffix(pa, "/ws") {
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

	switch fname {
	case "":
		ctx.Writer.Header().Set("Content-Type", "text/html")
		ctx.Writer.Write(webrtcIndex)
		return

	case "ws":
		wsconn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
		if err != nil {
			return
		}

		select {
		case s.connNew <- webRTCConnNewReq{
			pathName: dir,
			wsconn:   wsconn,
		}:
		case <-s.ctx.Done():
		}
	}
}

// connClose is called by webRTCConn.
func (s *webRTCServer) connClose(c *webRTCConn) {
	select {
	case s.chConnClose <- c:
	case <-s.ctx.Done():
	}
}
