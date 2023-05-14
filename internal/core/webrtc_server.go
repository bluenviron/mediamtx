package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/tls"
	_ "embed"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pion/ice/v2"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"

	"github.com/aler9/mediamtx/internal/conf"
	"github.com/aler9/mediamtx/internal/logger"
	"github.com/aler9/mediamtx/internal/websocket"
)

//go:embed webrtc_publish_index.html
var webrtcPublishIndex []byte

//go:embed webrtc_read_index.html
var webrtcReadIndex []byte

func parseICEFragment(buf []byte) ([]*webrtc.ICECandidateInit, error) {
	buf = append([]byte("v=0\r\no=- 0 0 IN IP4 0.0.0.0\r\ns=-\r\nt=0 0\r\n"), buf...)

	var sdp sdp.SessionDescription
	err := sdp.Unmarshal(buf)
	if err != nil {
		return nil, err
	}

	usernameFragment, ok := sdp.Attribute("ice-ufrag")
	if !ok {
		return nil, fmt.Errorf("ice-ufrag attribute is missing")
	}

	var ret []*webrtc.ICECandidateInit

	for _, media := range sdp.MediaDescriptions {
		mid, ok := media.Attribute("mid")
		if !ok {
			return nil, fmt.Errorf("mid attribute is missing")
		}

		tmp, err := strconv.ParseUint(mid, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid mid attribute")
		}
		midNum := uint16(tmp)

		for _, attr := range media.Attributes {
			if attr.Key == "candidate" {
				ret = append(ret, &webrtc.ICECandidateInit{
					Candidate:        attr.Value,
					SDPMid:           &mid,
					SDPMLineIndex:    &midNum,
					UsernameFragment: &usernameFragment,
				})
			}
		}
	}

	return ret, nil
}

type webRTCServerAPIConnsListItem struct {
	Created                   time.Time `json:"created"`
	RemoteAddr                string    `json:"remoteAddr"`
	PeerConnectionEstablished bool      `json:"peerConnectionEstablished"`
	LocalCandidate            string    `json:"localCandidate"`
	RemoteCandidate           string    `json:"remoteCandidate"`
	State                     string    `json:"state"`
	BytesReceived             uint64    `json:"bytesReceived"`
	BytesSent                 uint64    `json:"bytesSent"`
}

type webRTCServerAPIConnsListData struct {
	Items map[string]webRTCServerAPIConnsListItem `json:"items"`
}

type webRTCServerAPIConnsListRes struct {
	data *webRTCServerAPIConnsListData
	err  error
}

type webRTCServerAPIConnsListReq struct {
	res chan webRTCServerAPIConnsListRes
}

type webRTCServerAPIConnsKickRes struct {
	err error
}

type webRTCServerAPIConnsKickReq struct {
	id  string
	res chan webRTCServerAPIConnsKickRes
}

type webRTCNewConnReq struct {
	pathName     string
	publish      bool
	wsconn       *websocket.ServerConn
	res          chan *webRTCConn
	videoCodec   string
	audioCodec   string
	videoBitrate string
}

type webRTCNewSessionWHEPRes struct {
	sx     *webRTCSessionWHEP
	answer []byte
	err    error
}

type webRTCNewSessionWHEPReq struct {
	pathName   string
	remoteAddr string
	offer      []byte
	res        chan webRTCNewSessionWHEPRes
}

type webRTCSessionWHEPRemoteCandidatesRes struct {
	sx  *webRTCSessionWHEP
	err error
}

type webRTCSessionWHEPRemoteCandidatesReq struct {
	secret     uuid.UUID
	candidates []*webrtc.ICECandidateInit
	res        chan webRTCSessionWHEPRemoteCandidatesRes
}

type webRTCServerParent interface {
	logger.Writer
}

type webRTCServer struct {
	allowOrigin     string
	trustedProxies  conf.IPsOrCIDRs
	iceServers      []string
	readBufferCount int
	pathManager     *pathManager
	metrics         *metrics
	parent          webRTCServerParent

	ctx                  context.Context
	ctxCancel            func()
	ln                   net.Listener
	requestPool          *httpRequestPool
	httpServer           *http.Server
	udpMuxLn             net.PacketConn
	tcpMuxLn             net.Listener
	conns                map[*webRTCConn]struct{}
	sessionsWHEP         map[*webRTCSessionWHEP]struct{}
	sessionsWHEPBySecret map[uuid.UUID]*webRTCSessionWHEP
	iceHostNAT1To1IPs    []string
	iceUDPMux            ice.UDPMux
	iceTCPMux            ice.TCPMux

	// in
	chConnNew               chan webRTCNewConnReq
	chConnClose             chan *webRTCConn
	chSessionWHEPNew        chan webRTCNewSessionWHEPReq
	chSessionWHEPClose      chan *webRTCSessionWHEP
	chSessionWHEPCandidates chan webRTCSessionWHEPRemoteCandidatesReq
	chAPIConnsList          chan webRTCServerAPIConnsListReq
	chAPIConnsKick          chan webRTCServerAPIConnsKickReq

	// out
	done chan struct{}
}

func newWebRTCServer(
	parentCtx context.Context,
	address string,
	encryption bool,
	serverKey string,
	serverCert string,
	allowOrigin string,
	trustedProxies conf.IPsOrCIDRs,
	iceServers []string,
	readTimeout conf.StringDuration,
	readBufferCount int,
	pathManager *pathManager,
	metrics *metrics,
	parent webRTCServerParent,
	iceHostNAT1To1IPs []string,
	iceUDPMuxAddress string,
	iceTCPMuxAddress string,
) (*webRTCServer, error) {
	ln, err := net.Listen(restrictNetwork("tcp", address))
	if err != nil {
		return nil, err
	}

	var tlsConfig *tls.Config
	if encryption {
		crt, err := tls.LoadX509KeyPair(serverCert, serverKey)
		if err != nil {
			ln.Close()
			return nil, err
		}

		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{crt},
		}
	}

	var iceUDPMux ice.UDPMux
	var udpMuxLn net.PacketConn
	if iceUDPMuxAddress != "" {
		udpMuxLn, err = net.ListenPacket(restrictNetwork("udp", iceUDPMuxAddress))
		if err != nil {
			return nil, err
		}
		iceUDPMux = webrtc.NewICEUDPMux(nil, udpMuxLn)
	}

	var iceTCPMux ice.TCPMux
	var tcpMuxLn net.Listener
	if iceTCPMuxAddress != "" {
		tcpMuxLn, err = net.Listen(restrictNetwork("tcp", iceTCPMuxAddress))
		if err != nil {
			return nil, err
		}
		iceTCPMux = webrtc.NewICETCPMux(nil, tcpMuxLn, 8)
	}

	ctx, ctxCancel := context.WithCancel(parentCtx)

	s := &webRTCServer{
		allowOrigin:             allowOrigin,
		trustedProxies:          trustedProxies,
		iceServers:              iceServers,
		readBufferCount:         readBufferCount,
		pathManager:             pathManager,
		metrics:                 metrics,
		parent:                  parent,
		ctx:                     ctx,
		ctxCancel:               ctxCancel,
		ln:                      ln,
		udpMuxLn:                udpMuxLn,
		tcpMuxLn:                tcpMuxLn,
		iceUDPMux:               iceUDPMux,
		iceTCPMux:               iceTCPMux,
		iceHostNAT1To1IPs:       iceHostNAT1To1IPs,
		conns:                   make(map[*webRTCConn]struct{}),
		sessionsWHEP:            make(map[*webRTCSessionWHEP]struct{}),
		sessionsWHEPBySecret:    make(map[uuid.UUID]*webRTCSessionWHEP),
		chConnNew:               make(chan webRTCNewConnReq),
		chConnClose:             make(chan *webRTCConn),
		chSessionWHEPNew:        make(chan webRTCNewSessionWHEPReq),
		chSessionWHEPClose:      make(chan *webRTCSessionWHEP),
		chSessionWHEPCandidates: make(chan webRTCSessionWHEPRemoteCandidatesReq),
		chAPIConnsList:          make(chan webRTCServerAPIConnsListReq),
		chAPIConnsKick:          make(chan webRTCServerAPIConnsKickReq),
		done:                    make(chan struct{}),
	}

	s.requestPool = newHTTPRequestPool()

	router := gin.New()
	httpSetTrustedProxies(router, trustedProxies)

	router.NoRoute(s.requestPool.mw, httpLoggerMiddleware(s), httpServerHeaderMiddleware, s.onRequest)

	s.httpServer = &http.Server{
		Handler:           router,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: time.Duration(readTimeout),
		ErrorLog:          log.New(&nilWriter{}, "", 0),
	}

	str := "listener opened on " + address + " (HTTP)"
	if udpMuxLn != nil {
		str += ", " + iceUDPMuxAddress + " (ICE/UDP)"
	}
	if tcpMuxLn != nil {
		str += ", " + iceTCPMuxAddress + " (ICE/TCP)"
	}
	s.Log(logger.Info, str)

	if s.metrics != nil {
		s.metrics.webRTCServerSet(s)
	}

	go s.run()

	return s, nil
}

// Log is the main logging function.
func (s *webRTCServer) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[WebRTC] "+format, append([]interface{}{}, args...)...)
}

func (s *webRTCServer) close() {
	s.Log(logger.Info, "listener is closing")
	s.ctxCancel()
	<-s.done
}

func (s *webRTCServer) run() {
	defer close(s.done)

	if s.httpServer.TLSConfig != nil {
		go s.httpServer.ServeTLS(s.ln, "", "")
	} else {
		go s.httpServer.Serve(s.ln)
	}

	var wg sync.WaitGroup

outer:
	for {
		select {
		case req := <-s.chConnNew:
			c := newWebRTCConn(
				s.ctx,
				s.readBufferCount,
				req.pathName,
				req.publish,
				req.wsconn,
				req.videoCodec,
				req.audioCodec,
				req.videoBitrate,
				&wg,
				s.pathManager,
				s,
				s.iceHostNAT1To1IPs,
				s.iceUDPMux,
				s.iceTCPMux,
			)
			s.conns[c] = struct{}{}
			req.res <- c

		case conn := <-s.chConnClose:
			delete(s.conns, conn)

		case req := <-s.chSessionWHEPNew:
			sx := newWebRTCSessionWHEP(
				s.ctx,
				s.readBufferCount,
				req,
				&wg,
				s.iceHostNAT1To1IPs,
				s.iceUDPMux,
				s.iceTCPMux,
				s.pathManager,
				s,
			)
			s.sessionsWHEP[sx] = struct{}{}
			s.sessionsWHEPBySecret[sx.secret] = sx
			req.res <- webRTCNewSessionWHEPRes{sx: sx}

		case sx := <-s.chSessionWHEPClose:
			delete(s.sessionsWHEP, sx)
			delete(s.sessionsWHEPBySecret, sx.secret)

		case req := <-s.chSessionWHEPCandidates:
			sx, ok := s.sessionsWHEPBySecret[req.secret]
			if !ok {
				req.res <- webRTCSessionWHEPRemoteCandidatesRes{err: fmt.Errorf("session not found")}
			}

			req.res <- webRTCSessionWHEPRemoteCandidatesRes{sx: sx}

		case req := <-s.chAPIConnsList:
			data := &webRTCServerAPIConnsListData{
				Items: make(map[string]webRTCServerAPIConnsListItem),
			}

			for c := range s.conns {
				peerConnectionEstablished := false
				localCandidate := ""
				remoteCandidate := ""
				bytesReceived := uint64(0)
				bytesSent := uint64(0)

				pc := c.safePC()
				if pc != nil {
					peerConnectionEstablished = true
					localCandidate = pc.localCandidate()
					remoteCandidate = pc.remoteCandidate()
					bytesReceived = pc.bytesReceived()
					bytesSent = pc.bytesSent()
				}

				data.Items[c.uuid.String()] = webRTCServerAPIConnsListItem{
					Created:                   c.created,
					RemoteAddr:                c.remoteAddr().String(),
					PeerConnectionEstablished: peerConnectionEstablished,
					LocalCandidate:            localCandidate,
					RemoteCandidate:           remoteCandidate,
					State: func() string {
						if c.publish {
							return "publish"
						}
						return "read"
					}(),
					BytesReceived: bytesReceived,
					BytesSent:     bytesSent,
				}
			}

			req.res <- webRTCServerAPIConnsListRes{data: data}

		case req := <-s.chAPIConnsKick:
			res := func() bool {
				for c := range s.conns {
					if c.uuid.String() == req.id {
						delete(s.conns, c)
						c.close()
						return true
					}
				}
				return false
			}()
			if res {
				req.res <- webRTCServerAPIConnsKickRes{}
			} else {
				req.res <- webRTCServerAPIConnsKickRes{fmt.Errorf("not found")}
			}

		case <-s.ctx.Done():
			break outer
		}
	}

	s.ctxCancel()

	s.httpServer.Shutdown(context.Background())
	s.ln.Close() // in case Shutdown() is called before Serve()

	fmt.Println("waitA")
	s.requestPool.close()
	fmt.Println("waitB")
	wg.Wait()
	fmt.Println("waitC")

	if s.udpMuxLn != nil {
		s.udpMuxLn.Close()
	}

	if s.tcpMuxLn != nil {
		s.tcpMuxLn.Close()
	}
}

func (s *webRTCServer) genICEServers() []webrtc.ICEServer {
	ret := make([]webrtc.ICEServer, len(s.iceServers))
	for i, s := range s.iceServers {
		parts := strings.Split(s, ":")
		if len(parts) == 5 {
			if parts[1] == "AUTH_SECRET" {
				s := webrtc.ICEServer{
					URLs: []string{parts[0] + ":" + parts[3] + ":" + parts[4]},
				}

				randomUser := func() string {
					const charset = "abcdefghijklmnopqrstuvwxyz1234567890"
					b := make([]byte, 20)
					for i := range b {
						b[i] = charset[rand.Intn(len(charset))]
					}
					return string(b)
				}()

				expireDate := time.Now().Add(24 * 3600 * time.Second).Unix()
				s.Username = strconv.FormatInt(expireDate, 10) + ":" + randomUser

				h := hmac.New(sha1.New, []byte(parts[2]))
				h.Write([]byte(s.Username))
				s.Credential = base64.StdEncoding.EncodeToString(h.Sum(nil))

				ret[i] = s
			} else {
				ret[i] = webrtc.ICEServer{
					URLs:       []string{parts[0] + ":" + parts[3] + ":" + parts[4]},
					Username:   parts[1],
					Credential: parts[2],
				}
			}
		} else {
			ret[i] = webrtc.ICEServer{
				URLs: []string{s},
			}
		}
	}
	return ret
}

func (s *webRTCServer) genICEServersForLink() []string {
	servers := s.genICEServers()
	ret := make([]string, len(servers))

	for i, server := range servers {
		link := "<" + server.URLs[0] + ">; rel=\"ice-server\""
		if server.Username != "" {
			link += "; username=\"" + server.Username + "\"" +
				"; credential=\"" + server.Credential.(string) + "\"; credential-type=\"password\""
		}
		ret[i] = link
	}

	return ret
}

// newConn is called by onRequest.
func (s *webRTCServer) newConn(req webRTCNewConnReq) *webRTCConn {
	req.res = make(chan *webRTCConn)

	select {
	case s.chConnNew <- req:
		return <-req.res
	case <-s.ctx.Done():
		return nil
	}
}

// connClose is called by webRTCConn.
func (s *webRTCServer) connClose(c *webRTCConn) {
	select {
	case s.chConnClose <- c:
	case <-s.ctx.Done():
	}
}

// newSessionWHEP is called by onRequest.
func (s *webRTCServer) newSessionWHEP(req webRTCNewSessionWHEPReq) webRTCNewSessionWHEPRes {
	req.res = make(chan webRTCNewSessionWHEPRes)

	select {
	case s.chSessionWHEPNew <- req:
		res1 := <-req.res

		select {
		case res2 := <-req.res:
			return res2

		case <-res1.sx.ctx.Done():
			return webRTCNewSessionWHEPRes{err: fmt.Errorf("terminated")}
		}

	case <-s.ctx.Done():
		return webRTCNewSessionWHEPRes{err: fmt.Errorf("terminated")}
	}
}

// sessionWHEPClose is called by webRTCSessionWhHEP.
func (s *webRTCServer) sessionWHEPClose(sx *webRTCSessionWHEP) {
	select {
	case s.chSessionWHEPClose <- sx:
	case <-s.ctx.Done():
	}
}

func (s *webRTCServer) sessionWHEPCandidates(
	req webRTCSessionWHEPRemoteCandidatesReq,
) webRTCSessionWHEPRemoteCandidatesRes {
	req.res = make(chan webRTCSessionWHEPRemoteCandidatesRes)
	select {
	case s.chSessionWHEPCandidates <- req:
		res1 := <-req.res
		if res1.err != nil {
			return res1
		}

		return res1.sx.addRemoteCandidates(req)

	case <-s.ctx.Done():
		return webRTCSessionWHEPRemoteCandidatesRes{err: fmt.Errorf("terminated")}
	}
}

// apiConnsList is called by api.
func (s *webRTCServer) apiConnsList() webRTCServerAPIConnsListRes {
	req := webRTCServerAPIConnsListReq{
		res: make(chan webRTCServerAPIConnsListRes),
	}

	select {
	case s.chAPIConnsList <- req:
		return <-req.res

	case <-s.ctx.Done():
		return webRTCServerAPIConnsListRes{err: fmt.Errorf("terminated")}
	}
}

// apiConnsKick is called by api.
func (s *webRTCServer) apiConnsKick(id string) webRTCServerAPIConnsKickRes {
	req := webRTCServerAPIConnsKickReq{
		id:  id,
		res: make(chan webRTCServerAPIConnsKickRes),
	}

	select {
	case s.chAPIConnsKick <- req:
		return <-req.res

	case <-s.ctx.Done():
		return webRTCServerAPIConnsKickRes{err: fmt.Errorf("terminated")}
	}
}

func (s *webRTCServer) onRequest(ctx *gin.Context) {
	ctx.Writer.Header().Set("Access-Control-Allow-Origin", s.allowOrigin)
	ctx.Writer.Header().Set("Access-Control-Allow-Credentials", "true")

	// remove leading prefix
	pa := ctx.Request.URL.Path[1:]

	if !strings.HasSuffix(pa, "/whep") {
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
	}

	var dir string
	var fname string
	var publish bool

	switch {
	case pa == "favicon.ico":
		return

	case strings.HasSuffix(pa, "/publish/ws"):
		dir, fname = pa[:len(pa)-len("/publish/ws")], "publish/ws"
		publish = true

	case strings.HasSuffix(pa, "/publish"):
		dir, fname = pa[:len(pa)-len("/publish")], "publish"
		publish = true

	case strings.HasSuffix(pa, "/whep"):
		dir, fname = pa[:len(pa)-len("/whep")], "whep"
		publish = false

	default:
		dir, fname = pa, ""
		publish = false

		if !strings.HasSuffix(dir, "/") {
			ctx.Writer.Header().Set("Location", "/"+dir+"/")
			ctx.Writer.WriteHeader(http.StatusMovedPermanently)
			return
		}
	}

	dir = strings.TrimSuffix(dir, "/")
	if dir == "" {
		return
	}

	user, pass, hasCredentials := ctx.Request.BasicAuth()

	res := s.pathManager.getPathConf(pathGetPathConfReq{
		name:    dir,
		publish: publish,
		credentials: authCredentials{
			query: ctx.Request.URL.RawQuery,
			ip:    net.ParseIP(ctx.ClientIP()),
			user:  user,
			pass:  pass,
			proto: authProtocolWebRTC,
		},
	})
	if res.err != nil {
		if terr, ok := res.err.(pathErrAuth); ok {
			if !hasCredentials {
				ctx.Header("WWW-Authenticate", `Basic realm="mediamtx"`)
				ctx.Writer.WriteHeader(http.StatusUnauthorized)
				return
			}

			s.Log(logger.Info, "authentication error: %v", terr.wrapped)
			ctx.Writer.WriteHeader(http.StatusUnauthorized)
			return
		}

		ctx.Writer.WriteHeader(http.StatusNotFound)
		return
	}

	switch fname {
	case "":
		ctx.Writer.Header().Set("Content-Type", "text/html")
		ctx.Writer.WriteHeader(http.StatusOK)
		ctx.Writer.Write(webrtcReadIndex)

	case "whep":
		switch ctx.Request.Method {
		case http.MethodOptions:
			ctx.Writer.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			ctx.Writer.Header().Set("Access-Control-Allow-Headers", ctx.Request.Header.Get("Access-Control-Request-Headers"))
			ctx.Writer.Header()["Link"] = s.genICEServersForLink()
			ctx.Writer.WriteHeader(http.StatusOK)

		case http.MethodPost:
			if ctx.Request.Header.Get("Content-Type") != "application/sdp" {
				ctx.Writer.WriteHeader(http.StatusBadRequest)
				return
			}

			offer, err := io.ReadAll(ctx.Request.Body)
			if err != nil {
				return
			}

			res := s.newSessionWHEP(webRTCNewSessionWHEPReq{
				pathName:   dir,
				remoteAddr: ctx.ClientIP(),
				offer:      offer,
			})
			if res.err != nil {
				ctx.Writer.WriteHeader(http.StatusInternalServerError)
				return
			}

			ctx.Writer.Header().Set("Content-Type", "application/sdp")
			ctx.Writer.Header().Set("E-Tag", res.sx.secret.String())
			ctx.Writer.Header().Set("Accept-Patch", "application/trickle-ice-sdpfrag")
			ctx.Writer.WriteHeader(http.StatusCreated)
			ctx.Writer.Write(res.answer)

		case http.MethodPatch:
			secret, err := uuid.Parse(ctx.Request.Header.Get("If-Match"))
			if err != nil {
				ctx.Writer.WriteHeader(http.StatusBadRequest)
				return
			}

			if ctx.Request.Header.Get("Content-Type") != "application/trickle-ice-sdpfrag" {
				ctx.Writer.WriteHeader(http.StatusBadRequest)
				return
			}

			byts, err := io.ReadAll(ctx.Request.Body)
			if err != nil {
				return
			}

			candidates, err := parseICEFragment(byts)
			if err != nil {
				ctx.Writer.WriteHeader(http.StatusBadRequest)
				return
			}

			res := s.sessionWHEPCandidates(webRTCSessionWHEPRemoteCandidatesReq{
				secret:     secret,
				candidates: candidates,
			})
			if res.err != nil {
				ctx.Writer.WriteHeader(http.StatusBadRequest)
				return
			}

			ctx.Writer.WriteHeader(http.StatusNoContent)
		}

	case "publish":
		ctx.Writer.Header().Set("Content-Type", "text/html")
		ctx.Writer.WriteHeader(http.StatusOK)
		ctx.Writer.Write(webrtcPublishIndex)

	case "publish/ws":
		wsconn, err := websocket.NewServerConn(ctx.Writer, ctx.Request)
		if err != nil {
			return
		}
		defer wsconn.Close()

		c := s.newConn(webRTCNewConnReq{
			pathName:     dir,
			publish:      (fname == "publish/ws"),
			wsconn:       wsconn,
			videoCodec:   ctx.Query("video_codec"),
			audioCodec:   ctx.Query("audio_codec"),
			videoBitrate: ctx.Query("video_bitrate"),
		})
		if c == nil {
			return
		}

		c.wait()
	}
}
