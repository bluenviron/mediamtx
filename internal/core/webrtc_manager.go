package core

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pion/logging"
	pwebrtc "github.com/pion/webrtc/v3"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/webrtc"
	"github.com/bluenviron/mediamtx/internal/restrictnetwork"
)

const (
	webrtcPauseAfterAuthError  = 2 * time.Second
	webrtcTurnSecretExpiration = 24 * 3600 * time.Second
	webrtcPayloadMaxSize       = 1188 // 1200 - 12 (RTP header)
)

type nilWriter struct{}

func (nilWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

var webrtcNilLogger = logging.NewDefaultLeveledLoggerForScope("", 0, &nilWriter{})

func randInt63() (int64, error) {
	var b [8]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return 0, err
	}

	return int64(uint64(b[0]&0b01111111)<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 |
		uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7])), nil
}

// https://cs.opensource.google/go/go/+/refs/tags/go1.20.4:src/math/rand/rand.go;l=119
func randInt63n(n int64) (int64, error) {
	if n&(n-1) == 0 { // n is power of two, can mask
		r, err := randInt63()
		if err != nil {
			return 0, err
		}
		return r & (n - 1), nil
	}

	max := int64((1 << 63) - 1 - (1<<63)%uint64(n))

	v, err := randInt63()
	if err != nil {
		return 0, err
	}

	for v > max {
		v, err = randInt63()
		if err != nil {
			return 0, err
		}
	}

	return v % n, nil
}

func randomTurnUser() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz1234567890"
	b := make([]byte, 20)
	for i := range b {
		j, err := randInt63n(int64(len(charset)))
		if err != nil {
			return "", err
		}

		b[i] = charset[int(j)]
	}

	return string(b), nil
}

type webRTCManagerAPISessionsListRes struct {
	data *defs.APIWebRTCSessionList
	err  error
}

type webRTCManagerAPISessionsListReq struct {
	res chan webRTCManagerAPISessionsListRes
}

type webRTCManagerAPISessionsGetRes struct {
	data *defs.APIWebRTCSession
	err  error
}

type webRTCManagerAPISessionsGetReq struct {
	uuid uuid.UUID
	res  chan webRTCManagerAPISessionsGetRes
}

type webRTCManagerAPISessionsKickRes struct {
	err error
}

type webRTCManagerAPISessionsKickReq struct {
	uuid uuid.UUID
	res  chan webRTCManagerAPISessionsKickRes
}

type webRTCNewSessionRes struct {
	sx            *webRTCSession
	answer        []byte
	errStatusCode int
	err           error
}

type webRTCNewSessionReq struct {
	pathName   string
	remoteAddr string
	query      string
	user       string
	pass       string
	offer      []byte
	publish    bool
	res        chan webRTCNewSessionRes
}

type webRTCAddSessionCandidatesRes struct {
	sx  *webRTCSession
	err error
}

type webRTCAddSessionCandidatesReq struct {
	secret     uuid.UUID
	candidates []*pwebrtc.ICECandidateInit
	res        chan webRTCAddSessionCandidatesRes
}

type webRTCDeleteSessionRes struct {
	err error
}

type webRTCDeleteSessionReq struct {
	secret uuid.UUID
	res    chan webRTCDeleteSessionRes
}

type webRTCManagerParent interface {
	logger.Writer
}

type webRTCManager struct {
	Address               string
	Encryption            bool
	ServerKey             string
	ServerCert            string
	AllowOrigin           string
	TrustedProxies        conf.IPsOrCIDRs
	ReadTimeout           conf.StringDuration
	WriteQueueSize        int
	LocalUDPAddress       string
	LocalTCPAddress       string
	IPsFromInterfaces     bool
	IPsFromInterfacesList []string
	AdditionalHosts       []string
	ICEServers            []conf.WebRTCICEServer
	ExternalCmdPool       *externalcmd.Pool
	PathManager           *pathManager
	Parent                webRTCManagerParent

	ctx              context.Context
	ctxCancel        func()
	httpServer       *webRTCHTTPServer
	udpMuxLn         net.PacketConn
	tcpMuxLn         net.Listener
	api              *pwebrtc.API
	sessions         map[*webRTCSession]struct{}
	sessionsBySecret map[uuid.UUID]*webRTCSession

	// in
	chNewSession           chan webRTCNewSessionReq
	chCloseSession         chan *webRTCSession
	chAddSessionCandidates chan webRTCAddSessionCandidatesReq
	chDeleteSession        chan webRTCDeleteSessionReq
	chAPISessionsList      chan webRTCManagerAPISessionsListReq
	chAPISessionsGet       chan webRTCManagerAPISessionsGetReq
	chAPIConnsKick         chan webRTCManagerAPISessionsKickReq

	// out
	done chan struct{}
}

func (m *webRTCManager) initialize() error {
	ctx, ctxCancel := context.WithCancel(context.Background())

	m.ctx = ctx
	m.ctxCancel = ctxCancel
	m.sessions = make(map[*webRTCSession]struct{})
	m.sessionsBySecret = make(map[uuid.UUID]*webRTCSession)
	m.chNewSession = make(chan webRTCNewSessionReq)
	m.chCloseSession = make(chan *webRTCSession)
	m.chAddSessionCandidates = make(chan webRTCAddSessionCandidatesReq)
	m.chDeleteSession = make(chan webRTCDeleteSessionReq)
	m.chAPISessionsList = make(chan webRTCManagerAPISessionsListReq)
	m.chAPISessionsGet = make(chan webRTCManagerAPISessionsGetReq)
	m.chAPIConnsKick = make(chan webRTCManagerAPISessionsKickReq)
	m.done = make(chan struct{})

	var err error
	m.httpServer, err = newWebRTCHTTPServer(
		m.Address,
		m.Encryption,
		m.ServerKey,
		m.ServerCert,
		m.AllowOrigin,
		m.TrustedProxies,
		m.ReadTimeout,
		m.PathManager,
		m,
	)
	if err != nil {
		ctxCancel()
		return err
	}

	apiConf := webrtc.APIConf{
		LocalRandomUDP:        false,
		IPsFromInterfaces:     m.IPsFromInterfaces,
		IPsFromInterfacesList: m.IPsFromInterfacesList,
		AdditionalHosts:       m.AdditionalHosts,
	}

	if m.LocalUDPAddress != "" {
		m.udpMuxLn, err = net.ListenPacket(restrictnetwork.Restrict("udp", m.LocalUDPAddress))
		if err != nil {
			m.httpServer.close()
			ctxCancel()
			return err
		}
		apiConf.ICEUDPMux = pwebrtc.NewICEUDPMux(webrtcNilLogger, m.udpMuxLn)
	}

	if m.LocalTCPAddress != "" {
		m.tcpMuxLn, err = net.Listen(restrictnetwork.Restrict("tcp", m.LocalTCPAddress))
		if err != nil {
			m.udpMuxLn.Close()
			m.httpServer.close()
			ctxCancel()
			return err
		}
		apiConf.ICETCPMux = pwebrtc.NewICETCPMux(webrtcNilLogger, m.tcpMuxLn, 8)
	}

	m.api, err = webrtc.NewAPI(apiConf)
	if err != nil {
		m.udpMuxLn.Close()
		m.tcpMuxLn.Close()
		m.httpServer.close()
		ctxCancel()
		return err
	}

	str := "listener opened on " + m.Address + " (HTTP)"
	if m.udpMuxLn != nil {
		str += ", " + m.LocalUDPAddress + " (ICE/UDP)"
	}
	if m.tcpMuxLn != nil {
		str += ", " + m.LocalTCPAddress + " (ICE/TCP)"
	}
	m.Log(logger.Info, str)

	go m.run()

	return nil
}

// Log is the main logging function.
func (m *webRTCManager) Log(level logger.Level, format string, args ...interface{}) {
	m.Parent.Log(level, "[WebRTC] "+format, args...)
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
		case req := <-m.chNewSession:
			sx := newWebRTCSession(
				m.ctx,
				m.WriteQueueSize,
				m.api,
				req,
				&wg,
				m.ExternalCmdPool,
				m.PathManager,
				m,
			)
			m.sessions[sx] = struct{}{}
			m.sessionsBySecret[sx.secret] = sx
			req.res <- webRTCNewSessionRes{sx: sx}

		case sx := <-m.chCloseSession:
			delete(m.sessions, sx)
			delete(m.sessionsBySecret, sx.secret)

		case req := <-m.chAddSessionCandidates:
			sx, ok := m.sessionsBySecret[req.secret]
			if !ok {
				req.res <- webRTCAddSessionCandidatesRes{err: fmt.Errorf("session not found")}
				continue
			}

			req.res <- webRTCAddSessionCandidatesRes{sx: sx}

		case req := <-m.chDeleteSession:
			sx, ok := m.sessionsBySecret[req.secret]
			if !ok {
				req.res <- webRTCDeleteSessionRes{err: fmt.Errorf("session not found")}
				continue
			}

			delete(m.sessions, sx)
			delete(m.sessionsBySecret, sx.secret)
			sx.close()

			req.res <- webRTCDeleteSessionRes{}

		case req := <-m.chAPISessionsList:
			data := &defs.APIWebRTCSessionList{
				Items: []*defs.APIWebRTCSession{},
			}

			for sx := range m.sessions {
				data.Items = append(data.Items, sx.apiItem())
			}

			sort.Slice(data.Items, func(i, j int) bool {
				return data.Items[i].Created.Before(data.Items[j].Created)
			})

			req.res <- webRTCManagerAPISessionsListRes{data: data}

		case req := <-m.chAPISessionsGet:
			sx := m.findSessionByUUID(req.uuid)
			if sx == nil {
				req.res <- webRTCManagerAPISessionsGetRes{err: fmt.Errorf("session not found")}
				continue
			}

			req.res <- webRTCManagerAPISessionsGetRes{data: sx.apiItem()}

		case req := <-m.chAPIConnsKick:
			sx := m.findSessionByUUID(req.uuid)
			if sx == nil {
				req.res <- webRTCManagerAPISessionsKickRes{err: fmt.Errorf("session not found")}
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

func (m *webRTCManager) generateICEServers() ([]pwebrtc.ICEServer, error) {
	ret := make([]pwebrtc.ICEServer, len(m.ICEServers))

	for i, server := range m.ICEServers {
		if server.Username == "AUTH_SECRET" {
			expireDate := time.Now().Add(webrtcTurnSecretExpiration).Unix()

			user, err := randomTurnUser()
			if err != nil {
				return nil, err
			}

			server.Username = strconv.FormatInt(expireDate, 10) + ":" + user

			h := hmac.New(sha1.New, []byte(server.Password))
			h.Write([]byte(server.Username))

			server.Password = base64.StdEncoding.EncodeToString(h.Sum(nil))
		}

		ret[i] = pwebrtc.ICEServer{
			URLs:       []string{server.URL},
			Username:   server.Username,
			Credential: server.Password,
		}
	}

	return ret, nil
}

// newSession is called by webRTCHTTPServer.
func (m *webRTCManager) newSession(req webRTCNewSessionReq) webRTCNewSessionRes {
	req.res = make(chan webRTCNewSessionRes)

	select {
	case m.chNewSession <- req:
		res := <-req.res

		return res.sx.new(req)

	case <-m.ctx.Done():
		return webRTCNewSessionRes{
			errStatusCode: http.StatusInternalServerError,
			err:           fmt.Errorf("terminated"),
		}
	}
}

// closeSession is called by webRTCSession.
func (m *webRTCManager) closeSession(sx *webRTCSession) {
	select {
	case m.chCloseSession <- sx:
	case <-m.ctx.Done():
	}
}

// addSessionCandidates is called by webRTCHTTPServer.
func (m *webRTCManager) addSessionCandidates(
	req webRTCAddSessionCandidatesReq,
) webRTCAddSessionCandidatesRes {
	req.res = make(chan webRTCAddSessionCandidatesRes)
	select {
	case m.chAddSessionCandidates <- req:
		res1 := <-req.res
		if res1.err != nil {
			return res1
		}

		return res1.sx.addCandidates(req)

	case <-m.ctx.Done():
		return webRTCAddSessionCandidatesRes{err: fmt.Errorf("terminated")}
	}
}

// deleteSession is called by webRTCHTTPServer.
func (m *webRTCManager) deleteSession(req webRTCDeleteSessionReq) error {
	req.res = make(chan webRTCDeleteSessionRes)
	select {
	case m.chDeleteSession <- req:
		res := <-req.res
		return res.err

	case <-m.ctx.Done():
		return fmt.Errorf("terminated")
	}
}

// apiSessionsList is called by api.
func (m *webRTCManager) apiSessionsList() (*defs.APIWebRTCSessionList, error) {
	req := webRTCManagerAPISessionsListReq{
		res: make(chan webRTCManagerAPISessionsListRes),
	}

	select {
	case m.chAPISessionsList <- req:
		res := <-req.res
		return res.data, res.err

	case <-m.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}

// apiSessionsGet is called by api.
func (m *webRTCManager) apiSessionsGet(uuid uuid.UUID) (*defs.APIWebRTCSession, error) {
	req := webRTCManagerAPISessionsGetReq{
		uuid: uuid,
		res:  make(chan webRTCManagerAPISessionsGetRes),
	}

	select {
	case m.chAPISessionsGet <- req:
		res := <-req.res
		return res.data, res.err

	case <-m.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}

// apiSessionsKick is called by api.
func (m *webRTCManager) apiSessionsKick(uuid uuid.UUID) error {
	req := webRTCManagerAPISessionsKickReq{
		uuid: uuid,
		res:  make(chan webRTCManagerAPISessionsKickRes),
	}

	select {
	case m.chAPIConnsKick <- req:
		res := <-req.res
		return res.err

	case <-m.ctx.Done():
		return fmt.Errorf("terminated")
	}
}
