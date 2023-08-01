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
	"github.com/pion/ice/v2"
	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v3"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
)

const (
	webrtcPauseAfterAuthError  = 2 * time.Second
	webrtcHandshakeTimeout     = 10 * time.Second
	webrtcTrackGatherTimeout   = 3 * time.Second
	webrtcPayloadMaxSize       = 1188 // 1200 - 12 (RTP header)
	webrtcStreamID             = "mediamtx"
	webrtcTurnSecretExpiration = 24 * 3600 * time.Second
)

var videoCodecs = []webrtc.RTPCodecParameters{
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeAV1,
			ClockRate: 90000,
		},
		PayloadType: 96,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeVP9,
			ClockRate:   90000,
			SDPFmtpLine: "profile-id=0",
		},
		PayloadType: 97,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeVP9,
			ClockRate:   90000,
			SDPFmtpLine: "profile-id=1",
		},
		PayloadType: 98,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeVP8,
			ClockRate: 90000,
		},
		PayloadType: 99,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f",
		},
		PayloadType: 100,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
		},
		PayloadType: 101,
	},
}

var audioCodecs = []webrtc.RTPCodecParameters{
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeOpus,
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "minptime=10;useinbandfec=1",
		},
		PayloadType: 111,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeG722,
			ClockRate: 8000,
		},
		PayloadType: 9,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMU,
			ClockRate: 8000,
		},
		PayloadType: 0,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMA,
			ClockRate: 8000,
		},
		PayloadType: 8,
	},
}

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

func webrtcNewAPI(
	iceHostNAT1To1IPs []string,
	iceUDPMux ice.UDPMux,
	iceTCPMux ice.TCPMux,
) (*webrtc.API, error) {
	settingsEngine := webrtc.SettingEngine{}

	if len(iceHostNAT1To1IPs) != 0 {
		settingsEngine.SetNAT1To1IPs(iceHostNAT1To1IPs, webrtc.ICECandidateTypeHost)
	}

	if iceUDPMux != nil {
		settingsEngine.SetICEUDPMux(iceUDPMux)
	}

	if iceTCPMux != nil {
		settingsEngine.SetICETCPMux(iceTCPMux)
		settingsEngine.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeTCP4})
	}

	mediaEngine := &webrtc.MediaEngine{}

	for _, codec := range videoCodecs {
		err := mediaEngine.RegisterCodec(codec, webrtc.RTPCodecTypeVideo)
		if err != nil {
			return nil, err
		}
	}

	for _, codec := range audioCodecs {
		err := mediaEngine.RegisterCodec(codec, webrtc.RTPCodecTypeAudio)
		if err != nil {
			return nil, err
		}
	}

	interceptorRegistry := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry); err != nil {
		return nil, err
	}

	return webrtc.NewAPI(
		webrtc.WithSettingEngine(settingsEngine),
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithInterceptorRegistry(interceptorRegistry)), nil
}

type webRTCManagerAPISessionsListRes struct {
	data *apiWebRTCSessionsList
	err  error
}

type webRTCManagerAPISessionsListReq struct {
	res chan webRTCManagerAPISessionsListRes
}

type webRTCManagerAPISessionsGetRes struct {
	data *apiWebRTCSession
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
	err           error
	errStatusCode int
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
	candidates []*webrtc.ICECandidateInit
	res        chan webRTCAddSessionCandidatesRes
}

type webRTCManagerParent interface {
	logger.Writer
}

type webRTCManager struct {
	allowOrigin     string
	trustedProxies  conf.IPsOrCIDRs
	iceServers      []conf.WebRTCICEServer
	readBufferCount int
	pathManager     *pathManager
	metrics         *metrics
	parent          webRTCManagerParent

	ctx              context.Context
	ctxCancel        func()
	httpServer       *webRTCHTTPServer
	udpMuxLn         net.PacketConn
	tcpMuxLn         net.Listener
	api              *webrtc.API
	sessions         map[*webRTCSession]struct{}
	sessionsBySecret map[uuid.UUID]*webRTCSession

	// in
	chNewSession           chan webRTCNewSessionReq
	chCloseSession         chan *webRTCSession
	chAddSessionCandidates chan webRTCAddSessionCandidatesReq
	chAPISessionsList      chan webRTCManagerAPISessionsListReq
	chAPISessionsGet       chan webRTCManagerAPISessionsGetReq
	chAPIConnsKick         chan webRTCManagerAPISessionsKickReq

	// out
	done chan struct{}
}

func newWebRTCManager(
	address string,
	encryption bool,
	serverKey string,
	serverCert string,
	allowOrigin string,
	trustedProxies conf.IPsOrCIDRs,
	iceServers []conf.WebRTCICEServer,
	readTimeout conf.StringDuration,
	readBufferCount int,
	iceHostNAT1To1IPs []string,
	iceUDPMuxAddress string,
	iceTCPMuxAddress string,
	pathManager *pathManager,
	metrics *metrics,
	parent webRTCManagerParent,
) (*webRTCManager, error) {
	ctx, ctxCancel := context.WithCancel(context.Background())

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
		sessions:               make(map[*webRTCSession]struct{}),
		sessionsBySecret:       make(map[uuid.UUID]*webRTCSession),
		chNewSession:           make(chan webRTCNewSessionReq),
		chCloseSession:         make(chan *webRTCSession),
		chAddSessionCandidates: make(chan webRTCAddSessionCandidatesReq),
		chAPISessionsList:      make(chan webRTCManagerAPISessionsListReq),
		chAPISessionsGet:       make(chan webRTCManagerAPISessionsGetReq),
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

	var iceUDPMux ice.UDPMux

	if iceUDPMuxAddress != "" {
		m.udpMuxLn, err = net.ListenPacket(restrictNetwork("udp", iceUDPMuxAddress))
		if err != nil {
			m.httpServer.close()
			ctxCancel()
			return nil, err
		}
		iceUDPMux = webrtc.NewICEUDPMux(nil, m.udpMuxLn)
	}

	var iceTCPMux ice.TCPMux

	if iceTCPMuxAddress != "" {
		m.tcpMuxLn, err = net.Listen(restrictNetwork("tcp", iceTCPMuxAddress))
		if err != nil {
			m.udpMuxLn.Close()
			m.httpServer.close()
			ctxCancel()
			return nil, err
		}
		iceTCPMux = webrtc.NewICETCPMux(nil, m.tcpMuxLn, 8)
	}

	m.api, err = webrtcNewAPI(iceHostNAT1To1IPs, iceUDPMux, iceTCPMux)
	if err != nil {
		m.udpMuxLn.Close()
		m.tcpMuxLn.Close()
		m.httpServer.close()
		ctxCancel()
		return nil, err
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
		case req := <-m.chNewSession:
			sx := newWebRTCSession(
				m.ctx,
				m.readBufferCount,
				m.api,
				req,
				&wg,
				m.pathManager,
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

		case req := <-m.chAPISessionsList:
			data := &apiWebRTCSessionsList{
				Items: []*apiWebRTCSession{},
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
				req.res <- webRTCManagerAPISessionsGetRes{err: errAPINotFound}
				continue
			}

			req.res <- webRTCManagerAPISessionsGetRes{data: sx.apiItem()}

		case req := <-m.chAPIConnsKick:
			sx := m.findSessionByUUID(req.uuid)
			if sx == nil {
				req.res <- webRTCManagerAPISessionsKickRes{err: errAPINotFound}
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

func (m *webRTCManager) generateICEServers() ([]webrtc.ICEServer, error) {
	ret := make([]webrtc.ICEServer, len(m.iceServers))

	for i, server := range m.iceServers {
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

		ret[i] = webrtc.ICEServer{
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
		return webRTCNewSessionRes{err: fmt.Errorf("terminated"), errStatusCode: http.StatusInternalServerError}
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

// apiSessionsList is called by api.
func (m *webRTCManager) apiSessionsList() (*apiWebRTCSessionsList, error) {
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
func (m *webRTCManager) apiSessionsGet(uuid uuid.UUID) (*apiWebRTCSession, error) {
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
