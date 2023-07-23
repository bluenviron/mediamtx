package core

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
)

//go:embed webrtc_publish_index.html
var webrtcPublishIndex []byte

//go:embed webrtc_read_index.html
var webrtcReadIndex []byte

func quoteCredential(v string) string {
	b, _ := json.Marshal(v)
	s := string(b)
	return s[1 : len(s)-1]
}

func unquoteCredential(v string) string {
	var s string
	json.Unmarshal([]byte("\""+v+"\""), &s)
	return s
}

func iceServersToLinkHeader(iceServers []webrtc.ICEServer) []string {
	ret := make([]string, len(iceServers))

	for i, server := range iceServers {
		link := "<" + server.URLs[0] + ">; rel=\"ice-server\""
		if server.Username != "" {
			link += "; username=\"" + quoteCredential(server.Username) + "\"" +
				"; credential=\"" + quoteCredential(server.Credential.(string)) + "\"; credential-type=\"password\""
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
				s.Username = unquoteCredential(m[3])
				s.Credential = unquoteCredential(m[4])
				s.CredentialType = webrtc.ICECredentialTypePassword
			}

			ret = append(ret, s)
		}
	}

	return ret
}

func unmarshalICEFragment(buf []byte) ([]*webrtc.ICECandidateInit, error) {
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

func marshalICEFragment(offer *webrtc.SessionDescription, candidates []*webrtc.ICECandidateInit) ([]byte, error) {
	var sdp sdp.SessionDescription
	err := sdp.Unmarshal([]byte(offer.SDP))
	if err != nil || len(sdp.MediaDescriptions) == 0 {
		return nil, err
	}

	firstMedia := sdp.MediaDescriptions[0]
	iceUfrag, _ := firstMedia.Attribute("ice-ufrag")
	icePwd, _ := firstMedia.Attribute("ice-pwd")

	candidatesByMedia := make(map[uint16][]*webrtc.ICECandidateInit)
	for _, candidate := range candidates {
		mid := *candidate.SDPMLineIndex
		candidatesByMedia[mid] = append(candidatesByMedia[mid], candidate)
	}

	frag := "a=ice-ufrag:" + iceUfrag + "\r\n" +
		"a=ice-pwd:" + icePwd + "\r\n"

	for mid, media := range sdp.MediaDescriptions {
		cbm, ok := candidatesByMedia[uint16(mid)]
		if ok {
			frag += "m=" + media.MediaName.String() + "\r\n" +
				"a=mid:" + strconv.FormatUint(uint64(mid), 10) + "\r\n"

			for _, candidate := range cbm {
				frag += "a=" + candidate.Candidate + "\r\n"
			}
		}
	}

	return []byte(frag), nil
}

type webRTCHTTPServerParent interface {
	logger.Writer
	generateICEServers() []webrtc.ICEServer
	sessionNew(req webRTCSessionNewReq) webRTCSessionNewRes
	sessionAddCandidates(req webRTCSessionAddCandidatesReq) webRTCSessionAddCandidatesRes
}

type webRTCHTTPServer struct {
	allowOrigin string
	pathManager *pathManager
	parent      webRTCHTTPServerParent

	inner *httpServer
}

func newWebRTCHTTPServer( //nolint:dupl
	address string,
	encryption bool,
	serverKey string,
	serverCert string,
	allowOrigin string,
	trustedProxies conf.IPsOrCIDRs,
	readTimeout conf.StringDuration,
	pathManager *pathManager,
	parent webRTCHTTPServerParent,
) (*webRTCHTTPServer, error) {
	if encryption {
		if serverCert == "" {
			return nil, fmt.Errorf("server cert is missing")
		}
	} else {
		serverKey = ""
		serverCert = ""
	}

	s := &webRTCHTTPServer{
		allowOrigin: allowOrigin,
		pathManager: pathManager,
		parent:      parent,
	}

	router := gin.New()
	httpSetTrustedProxies(router, trustedProxies)
	router.NoRoute(httpLoggerMiddleware(s), httpServerHeaderMiddleware, s.onRequest)

	var err error
	s.inner, err = newHTTPServer(
		address,
		readTimeout,
		serverCert,
		serverKey,
		router,
	)
	if err != nil {
		return nil, err
	}

	return s, nil
}

func (s *webRTCHTTPServer) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, format, args...)
}

func (s *webRTCHTTPServer) close() {
	s.inner.close()
}

func (s *webRTCHTTPServer) onRequest(ctx *gin.Context) {
	ctx.Writer.Header().Set("Access-Control-Allow-Origin", s.allowOrigin)
	ctx.Writer.Header().Set("Access-Control-Allow-Credentials", "true")

	// remove leading prefix
	pa := ctx.Request.URL.Path[1:]

	isWHIPorWHEP := strings.HasSuffix(pa, "/whip") || strings.HasSuffix(pa, "/whep")
	isPreflight := ctx.Request.Method == http.MethodOptions &&
		ctx.Request.Header.Get("Access-Control-Request-Method") != ""

	if !isWHIPorWHEP || isPreflight {
		switch ctx.Request.Method {
		case http.MethodOptions:
			ctx.Writer.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST, PATCH")
			ctx.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match")
			ctx.Writer.WriteHeader(http.StatusOK)
			return

		case http.MethodGet:

		default:
			return
		}
	}

	var dir string
	var fname string
	var publish bool

	switch {
	case pa == "", pa == "favicon.ico":
		return

	case strings.HasSuffix(pa, "/publish"):
		dir, fname = pa[:len(pa)-len("/publish")], "publish"
		publish = true

	case strings.HasSuffix(pa, "/whip"):
		dir, fname = pa[:len(pa)-len("/whip")], "whip"
		publish = true

	case strings.HasSuffix(pa, "/whep"):
		dir, fname = pa[:len(pa)-len("/whep")], "whep"
		publish = false

	default:
		dir, fname = pa, ""
		publish = false

		if !strings.HasSuffix(dir, "/") {
			l := "/" + dir + "/"
			if ctx.Request.URL.RawQuery != "" {
				l += "?" + ctx.Request.URL.RawQuery
			}
			ctx.Writer.Header().Set("Location", l)
			ctx.Writer.WriteHeader(http.StatusMovedPermanently)
			return
		}
	}

	dir = strings.TrimSuffix(dir, "/")
	if dir == "" {
		return
	}

	ip := ctx.ClientIP()
	_, port, _ := net.SplitHostPort(ctx.Request.RemoteAddr)

	user, pass, hasCredentials := ctx.Request.BasicAuth()

	res := s.pathManager.getConfForPath(pathGetConfForPathReq{
		name:    dir,
		publish: publish,
		credentials: authCredentials{
			query: ctx.Request.URL.RawQuery,
			ip:    net.ParseIP(ip),
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

	case "publish":
		ctx.Writer.Header().Set("Content-Type", "text/html")
		ctx.Writer.WriteHeader(http.StatusOK)
		ctx.Writer.Write(webrtcPublishIndex)

	case "whip", "whep":
		switch ctx.Request.Method {
		case http.MethodOptions:
			ctx.Writer.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST, PATCH")
			ctx.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match")
			ctx.Writer.Header()["Link"] = iceServersToLinkHeader(s.parent.generateICEServers())
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

			res := s.parent.sessionNew(webRTCSessionNewReq{
				pathName:   dir,
				remoteAddr: net.JoinHostPort(ip, port),
				offer:      offer,
				publish:    (fname == "whip"),
			})
			if res.err != nil {
				ctx.Writer.WriteHeader(res.errStatusCode)
				return
			}

			ctx.Writer.Header().Set("Content-Type", "application/sdp")
			ctx.Writer.Header().Set("Access-Control-Expose-Headers", "E-Tag, Accept-Patch, Link")
			ctx.Writer.Header().Set("E-Tag", res.sx.secret.String())
			ctx.Writer.Header().Set("ID", res.sx.uuid.String())
			ctx.Writer.Header().Set("Accept-Patch", "application/trickle-ice-sdpfrag")
			ctx.Writer.Header()["Link"] = iceServersToLinkHeader(s.parent.generateICEServers())
			ctx.Writer.Header().Set("Location", ctx.Request.URL.String())
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

			candidates, err := unmarshalICEFragment(byts)
			if err != nil {
				ctx.Writer.WriteHeader(http.StatusBadRequest)
				return
			}

			res := s.parent.sessionAddCandidates(webRTCSessionAddCandidatesReq{
				secret:     secret,
				candidates: candidates,
			})
			if res.err != nil {
				ctx.Writer.WriteHeader(http.StatusBadRequest)
				return
			}

			ctx.Writer.WriteHeader(http.StatusNoContent)
		}
	}
}
