package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pion/ice/v2"
	"github.com/pion/webrtc/v3"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
)

func iceServersToLinkHeader(iceServers []webrtc.ICEServer) []string {
	ret := make([]string, len(iceServers))

	for i, server := range iceServers {
		link := "<" + server.URLs[0] + ">; rel=\"ice-server\""
		if server.Username != "" {
			link += "; username=\"" + server.Username + "\"" +
				"; credential=\"" + server.Credential.(string) + "\"; credential-type=\"password\""
		}
		ret[i] = link
	}

	return ret
}

var reLink = regexp.MustCompile(`^<(.+?)>; rel="ice-server"(; username="(.+?)"` +
	`; credential="(.+?)"; credential-type="password")?`)

func linkHeaderToIceServers(link []string) []webrtc.ICEServer {
	var ret []webrtc.ICEServer

	for _, li := range link {
		m := reLink.FindStringSubmatch(li)
		if m != nil {
			s := webrtc.ICEServer{
				URLs: []string{m[1]},
			}

			if m[3] != "" {
				s.Username = m[3]
				s.Credential = m[4]
				s.CredentialType = webrtc.ICECredentialTypePassword
			}

			ret = append(ret, s)
		}
	}

	return ret
}

type webRTCManagerAPISessionsListItem struct {
	Created                   time.Time `json:"created"`
	RemoteAddr                string    `json:"remoteAddr"`
	PeerConnectionEstablished bool      `json:"peerConnectionEstablished"`
	LocalCandidate            string    `json:"localCandidate"`
	RemoteCandidate           string    `json:"remoteCandidate"`
	State                     string    `json:"state"`
	BytesReceived             uint64    `json:"bytesReceived"`
	BytesSent                 uint64    `json:"bytesSent"`
}

type webRTCManagerAPISessionsListData struct {
	Items map[string]webRTCManagerAPISessionsListItem `json:"items"`
}

type webRTCManagerAPISessionsListRes struct {
	data *webRTCManagerAPISessionsListData
	err  error
}

type webRTCManagerAPISessionsListReq struct {
	res chan webRTCManagerAPISessionsListRes
}

type webRTCManagerAPISessionsKickRes struct {
	err error
}

type webRTCManagerAPISessionsKickReq struct {
	uuid uuid.UUID
	res  chan webRTCManagerAPISessionsKickRes
}

type webRTCNewSessionRes struct {
	sx     *webRTCSession
	answer []byte
	err    error
}

type webRTCSessionNewReq struct {
	pathName     string
	remoteAddr   string
	offer        []byte
	publish      bool
	videoCodec   string
	audioCodec   string
	videoBitrate string
	res          chan webRTCNewSessionRes
}

type webRTCSessionAddCandidatesRes struct {
	sx  *webRTCSession
	err error
}

type webRTCSessionAddCandidatesReq struct {
	secret     uuid.UUID
	candidates []*webrtc.ICECandidateInit
	res        chan webRTCSessionAddCandidatesRes
}

type webRTCManagerParent interface {
	logger.Writer
}

type webRTCManager struct {
	allowOrigin     string
	trustedProxies  conf.IPsOrCIDRs
	iceServers      []string
	readBufferCount int
	pathManager     *pathManager
	metrics         *metrics
	parent          webRTCManagerParent

	ctx               context.Context
	ctxCancel         func()
	httpServer        *webRTCHTTPServer
	udpMuxLn          net.PacketConn
	tcpMuxLn          net.Listener
	sessions          map[*webRTCSession]struct{}
	sessionsBySecret  map[uuid.UUID]*webRTCSession
	iceHostNAT1To1IPs []string
	iceUDPMux         ice.UDPMux
	iceTCPMux         ice.TCPMux

	// in
	chSessionNew           chan webRTCSessionNewReq
	chSessionClose         chan *webRTCSession
	chSessionAddCandidates chan webRTCSessionAddCandidatesReq
	chAPISessionsList      chan webRTCManagerAPISessionsListReq
	chAPIConnsKick         chan webRTCManagerAPISessionsKickReq

	// out
	done chan struct{}
}

func newWebRTCManager(
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
	parent webRTCManagerParent,
	iceHostNAT1To1IPs []string,
	iceUDPMuxAddress string,
	iceTCPMuxAddress string,
) (*webRTCManager, error) {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	m := &webRTCManager{
		allowOrigin:            allowOrigin,
		trustedProxies:         trustedProxies,
		iceServers:             iceServers,
		readBufferCount:        readBufferCount,
		pathManager:            pathManager,
		metrics:                metrics,
		parent:                 parent,
		ctx:                    ctx,
		ctxCancel:              ctxCancel,
		iceHostNAT1To1IPs:      iceHostNAT1To1IPs,
		sessions:               make(map[*webRTCSession]struct{}),
		sessionsBySecret:       make(map[uuid.UUID]*webRTCSession),
		chSessionNew:           make(chan webRTCSessionNewReq),
		chSessionClose:         make(chan *webRTCSession),
		chSessionAddCandidates: make(chan webRTCSessionAddCandidatesReq),
		chAPISessionsList:      make(chan webRTCManagerAPISessionsListReq),
		chAPIConnsKick:         make(chan webRTCManagerAPISessionsKickReq),
		done:                   make(chan struct{}),
	}

	var err error
	m.httpServer, err = newWebRTCHTTPServer(
		address,
		encryption,
		serverKey,
		serverCert,
		allowOrigin,
		trustedProxies,
		readTimeout,
		pathManager,
		m,
	)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	if iceUDPMuxAddress != "" {
		m.udpMuxLn, err = net.ListenPacket(restrictNetwork("udp", iceUDPMuxAddress))
		if err != nil {
			m.httpServer.close()
			ctxCancel()
			return nil, err
		}
		m.iceUDPMux = webrtc.NewICEUDPMux(nil, m.udpMuxLn)
	}

	if iceTCPMuxAddress != "" {
		m.tcpMuxLn, err = net.Listen(restrictNetwork("tcp", iceTCPMuxAddress))
		if err != nil {
			m.udpMuxLn.Close()
			m.httpServer.close()
			ctxCancel()
			return nil, err
		}
		m.iceTCPMux = webrtc.NewICETCPMux(nil, m.tcpMuxLn, 8)
	}

	str := "listener opened on " + address + " (HTTP)"
	if m.udpMuxLn != nil {
		str += ", " + iceUDPMuxAddress + " (ICE/UDP)"
	}
	if m.tcpMuxLn != nil {
		str += ", " + iceTCPMuxAddress + " (ICE/TCP)"
	}
	m.Log(logger.Info, str)

	if m.metrics != nil {
		m.metrics.webRTCManagerSet(m)
	}

	go m.run()

	return m, nil
}

// Log is the main logging function.
func (m *webRTCManager) Log(level logger.Level, format string, args ...interface{}) {
	m.parent.Log(level, "[WebRTC] "+format, append([]interface{}{}, args...)...)
}

func (m *webRTCManager) close() {
	m.Log(logger.Info, "listener is closing")
	m.ctxCancel()
	<-m.done
}

func (m *webRTCManager) run() {
	defer close(m.done)

	var wg sync.WaitGroup

outer:
	for {
		select {
		case req := <-m.chSessionNew:
			sx := newWebRTCSession(
				m.ctx,
				m.readBufferCount,
				req,
				&wg,
				m.iceHostNAT1To1IPs,
				m.iceUDPMux,
				m.iceTCPMux,
				m.pathManager,
				m,
			)
			m.sessions[sx] = struct{}{}
			m.sessionsBySecret[sx.secret] = sx
			req.res <- webRTCNewSessionRes{sx: sx}

		case sx := <-m.chSessionClose:
			delete(m.sessions, sx)
			delete(m.sessionsBySecret, sx.secret)

		case req := <-m.chSessionAddCandidates:
			sx, ok := m.sessionsBySecret[req.secret]
			if !ok {
				req.res <- webRTCSessionAddCandidatesRes{err: fmt.Errorf("session not found")}
				continue
			}

			req.res <- webRTCSessionAddCandidatesRes{sx: sx}

		case req := <-m.chAPISessionsList:
			data := &webRTCManagerAPISessionsListData{
				Items: make(map[string]webRTCManagerAPISessionsListItem),
			}

			for sx := range m.sessions {
				peerConnectionEstablished := false
				localCandidate := ""
				remoteCandidate := ""
				bytesReceived := uint64(0)
				bytesSent := uint64(0)

				pc := sx.safePC()
				if pc != nil {
					peerConnectionEstablished = true
					localCandidate = pc.localCandidate()
					remoteCandidate = pc.remoteCandidate()
					bytesReceived = pc.bytesReceived()
					bytesSent = pc.bytesSent()
				}

				data.Items[sx.uuid.String()] = webRTCManagerAPISessionsListItem{
					Created:                   sx.created,
					RemoteAddr:                sx.req.remoteAddr,
					PeerConnectionEstablished: peerConnectionEstablished,
					LocalCandidate:            localCandidate,
					RemoteCandidate:           remoteCandidate,
					State: func() string {
						if sx.req.publish {
							return "publish"
						}
						return "read"
					}(),
					BytesReceived: bytesReceived,
					BytesSent:     bytesSent,
				}
			}

			req.res <- webRTCManagerAPISessionsListRes{data: data}

		case req := <-m.chAPIConnsKick:
			sx := m.findSessionByUUID(req.uuid)
			if sx == nil {
				req.res <- webRTCManagerAPISessionsKickRes{fmt.Errorf("not found")}
				continue
			}

			delete(m.sessions, sx)
			delete(m.sessionsBySecret, sx.secret)
			sx.close()
			req.res <- webRTCManagerAPISessionsKickRes{}

		case <-m.ctx.Done():
			break outer
		}
	}

	m.ctxCancel()

	wg.Wait()

	m.httpServer.close()

	if m.udpMuxLn != nil {
		m.udpMuxLn.Close()
	}

	if m.tcpMuxLn != nil {
		m.tcpMuxLn.Close()
	}
}

func (m *webRTCManager) findSessionByUUID(uuid uuid.UUID) *webRTCSession {
	for sx := range m.sessions {
		if sx.uuid == uuid {
			return sx
		}
	}
	return nil
}

func (m *webRTCManager) genICEServers() []webrtc.ICEServer {
	ret := make([]webrtc.ICEServer, len(m.iceServers))
	for i, s := range m.iceServers {
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

// sessionNew is called by webRTCHTTPServer.
func (m *webRTCManager) sessionNew(req webRTCSessionNewReq) webRTCNewSessionRes {
	req.res = make(chan webRTCNewSessionRes)

	select {
	case m.chSessionNew <- req:
		res1 := <-req.res

		select {
		case res2 := <-req.res:
			return res2

		case <-res1.sx.ctx.Done():
			return webRTCNewSessionRes{err: fmt.Errorf("terminated")}
		}

	case <-m.ctx.Done():
		return webRTCNewSessionRes{err: fmt.Errorf("terminated")}
	}
}

// sessionClose is called by webRTCSession.
func (m *webRTCManager) sessionClose(sx *webRTCSession) {
	select {
	case m.chSessionClose <- sx:
	case <-m.ctx.Done():
	}
}

// sessionAddCandidates is called by webRTCHTTPServer.
func (m *webRTCManager) sessionAddCandidates(
	req webRTCSessionAddCandidatesReq,
) webRTCSessionAddCandidatesRes {
	req.res = make(chan webRTCSessionAddCandidatesRes)
	select {
	case m.chSessionAddCandidates <- req:
		res1 := <-req.res
		if res1.err != nil {
			return res1
		}

		return res1.sx.addRemoteCandidates(req)

	case <-m.ctx.Done():
		return webRTCSessionAddCandidatesRes{err: fmt.Errorf("terminated")}
	}
}

// apiSessionsList is called by api.
func (m *webRTCManager) apiSessionsList() webRTCManagerAPISessionsListRes {
	req := webRTCManagerAPISessionsListReq{
		res: make(chan webRTCManagerAPISessionsListRes),
	}

	select {
	case m.chAPISessionsList <- req:
		return <-req.res

	case <-m.ctx.Done():
		return webRTCManagerAPISessionsListRes{err: fmt.Errorf("terminated")}
	}
}

// apiSessionsKick is called by api.
func (m *webRTCManager) apiSessionsKick(uuid uuid.UUID) webRTCManagerAPISessionsKickRes {
	req := webRTCManagerAPISessionsKickReq{
		uuid: uuid,
		res:  make(chan webRTCManagerAPISessionsKickRes),
	}

	select {
	case m.chAPIConnsKick <- req:
		return <-req.res

	case <-m.ctx.Done():
		return webRTCManagerAPISessionsKickRes{err: fmt.Errorf("terminated")}
	}
}
