package core

import (
	_ "embed"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	pwebrtc "github.com/pion/webrtc/v3"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpserv"
	"github.com/bluenviron/mediamtx/internal/protocols/webrtc"
	"github.com/bluenviron/mediamtx/internal/restrictnetwork"
)

//go:embed webrtc_publish_index.html
var webrtcPublishIndex []byte

//go:embed webrtc_read_index.html
var webrtcReadIndex []byte

var (
	reWHIPWHEPNoID   = regexp.MustCompile("^/(.+?)/(whip|whep)$")
	reWHIPWHEPWithID = regexp.MustCompile("^/(.+?)/(whip|whep)/(.+?)$")
)

func webrtcWriteError(ctx *gin.Context, statusCode int, err error) {
	ctx.JSON(statusCode, &defs.APIError{
		Error: err.Error(),
	})
}

func sessionLocation(publish bool, secret uuid.UUID) string {
	ret := ""
	if publish {
		ret += "whip"
	} else {
		ret += "whep"
	}
	ret += "/" + secret.String()
	return ret
}

type webRTCHTTPServerParent interface {
	logger.Writer
	generateICEServers() ([]pwebrtc.ICEServer, error)
	newSession(req webRTCNewSessionReq) webRTCNewSessionRes
	addSessionCandidates(req webRTCAddSessionCandidatesReq) webRTCAddSessionCandidatesRes
	deleteSession(req webRTCDeleteSessionReq) error
}

type webRTCHTTPServer struct {
	allowOrigin string
	pathManager *pathManager
	parent      webRTCHTTPServerParent

	inner *httpserv.WrappedServer
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
	router.SetTrustedProxies(trustedProxies.ToTrustedProxies()) //nolint:errcheck
	router.NoRoute(s.onRequest)

	network, address := restrictnetwork.Restrict("tcp", address)

	var err error
	s.inner, err = httpserv.NewWrappedServer(
		network,
		address,
		time.Duration(readTimeout),
		serverCert,
		serverKey,
		router,
		s,
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
	s.inner.Close()
}

func (s *webRTCHTTPServer) checkAuthOutsideSession(ctx *gin.Context, path string, publish bool) bool {
	ip := ctx.ClientIP()
	_, port, _ := net.SplitHostPort(ctx.Request.RemoteAddr)
	remoteAddr := net.JoinHostPort(ip, port)
	user, pass, hasCredentials := ctx.Request.BasicAuth()

	res := s.pathManager.getConfForPath(pathGetConfForPathReq{
		accessRequest: pathAccessRequest{
			name:    path,
			query:   ctx.Request.URL.RawQuery,
			publish: publish,
			ip:      net.ParseIP(ip),
			user:    user,
			pass:    pass,
			proto:   authProtocolWebRTC,
		},
	})
	if res.err != nil {
		if terr, ok := res.err.(*errAuthentication); ok {
			if !hasCredentials {
				ctx.Header("WWW-Authenticate", `Basic realm="mediamtx"`)
				ctx.Writer.WriteHeader(http.StatusUnauthorized)
				return false
			}

			s.Log(logger.Info, "connection %v failed to authenticate: %v", remoteAddr, terr.message)

			// wait some seconds to stop brute force attacks
			<-time.After(webrtcPauseAfterAuthError)

			webrtcWriteError(ctx, http.StatusUnauthorized, terr)
			return false
		}

		webrtcWriteError(ctx, http.StatusInternalServerError, res.err)
		return false
	}

	return true
}

func (s *webRTCHTTPServer) onWHIPOptions(ctx *gin.Context, path string, publish bool) {
	if !s.checkAuthOutsideSession(ctx, path, publish) {
		return
	}

	servers, err := s.parent.generateICEServers()
	if err != nil {
		webrtcWriteError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.Writer.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST, PATCH, DELETE")
	ctx.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match")
	ctx.Writer.Header().Set("Access-Control-Expose-Headers", "Link")
	ctx.Writer.Header()["Link"] = webrtc.LinkHeaderMarshal(servers)
	ctx.Writer.WriteHeader(http.StatusNoContent)
}

func (s *webRTCHTTPServer) onWHIPPost(ctx *gin.Context, path string, publish bool) {
	if ctx.Request.Header.Get("Content-Type") != "application/sdp" {
		webrtcWriteError(ctx, http.StatusBadRequest, fmt.Errorf("invalid Content-Type"))
		return
	}

	offer, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		return
	}

	ip := ctx.ClientIP()
	_, port, _ := net.SplitHostPort(ctx.Request.RemoteAddr)
	remoteAddr := net.JoinHostPort(ip, port)
	user, pass, _ := ctx.Request.BasicAuth()

	res := s.parent.newSession(webRTCNewSessionReq{
		pathName:   path,
		remoteAddr: remoteAddr,
		query:      ctx.Request.URL.RawQuery,
		user:       user,
		pass:       pass,
		offer:      offer,
		publish:    publish,
	})
	if res.err != nil {
		webrtcWriteError(ctx, res.errStatusCode, res.err)
		return
	}

	servers, err := s.parent.generateICEServers()
	if err != nil {
		webrtcWriteError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.Writer.Header().Set("Content-Type", "application/sdp")
	ctx.Writer.Header().Set("Access-Control-Expose-Headers", "ETag, ID, Accept-Patch, Link, Location")
	ctx.Writer.Header().Set("ETag", "*")
	ctx.Writer.Header().Set("ID", res.sx.uuid.String())
	ctx.Writer.Header().Set("Accept-Patch", "application/trickle-ice-sdpfrag")
	ctx.Writer.Header()["Link"] = webrtc.LinkHeaderMarshal(servers)
	ctx.Writer.Header().Set("Location", sessionLocation(publish, res.sx.secret))
	ctx.Writer.WriteHeader(http.StatusCreated)
	ctx.Writer.Write(res.answer)
}

func (s *webRTCHTTPServer) onWHIPPatch(ctx *gin.Context, rawSecret string) {
	secret, err := uuid.Parse(rawSecret)
	if err != nil {
		webrtcWriteError(ctx, http.StatusBadRequest, fmt.Errorf("invalid secret"))
		return
	}

	if ctx.Request.Header.Get("Content-Type") != "application/trickle-ice-sdpfrag" {
		webrtcWriteError(ctx, http.StatusBadRequest, fmt.Errorf("invalid Content-Type"))
		return
	}

	byts, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		return
	}

	candidates, err := webrtc.ICEFragmentUnmarshal(byts)
	if err != nil {
		webrtcWriteError(ctx, http.StatusBadRequest, err)
		return
	}

	res := s.parent.addSessionCandidates(webRTCAddSessionCandidatesReq{
		secret:     secret,
		candidates: candidates,
	})
	if res.err != nil {
		webrtcWriteError(ctx, http.StatusInternalServerError, res.err)
		return
	}

	ctx.Writer.WriteHeader(http.StatusNoContent)
}

func (s *webRTCHTTPServer) onWHIPDelete(ctx *gin.Context, rawSecret string) {
	secret, err := uuid.Parse(rawSecret)
	if err != nil {
		webrtcWriteError(ctx, http.StatusBadRequest, fmt.Errorf("invalid secret"))
		return
	}

	err = s.parent.deleteSession(webRTCDeleteSessionReq{
		secret: secret,
	})
	if err != nil {
		webrtcWriteError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.Writer.WriteHeader(http.StatusOK)
}

func (s *webRTCHTTPServer) onPage(ctx *gin.Context, path string, publish bool) {
	if !s.checkAuthOutsideSession(ctx, path, publish) {
		return
	}

	ctx.Writer.Header().Set("Cache-Control", "max-age=3600")
	ctx.Writer.Header().Set("Content-Type", "text/html")
	ctx.Writer.WriteHeader(http.StatusOK)

	if publish {
		ctx.Writer.Write(webrtcPublishIndex)
	} else {
		ctx.Writer.Write(webrtcReadIndex)
	}
}

func (s *webRTCHTTPServer) onRequest(ctx *gin.Context) {
	ctx.Writer.Header().Set("Access-Control-Allow-Origin", s.allowOrigin)
	ctx.Writer.Header().Set("Access-Control-Allow-Credentials", "true")

	// preflight requests
	if ctx.Request.Method == http.MethodOptions &&
		ctx.Request.Header.Get("Access-Control-Request-Method") != "" {
		ctx.Writer.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST, PATCH, DELETE")
		ctx.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match")
		ctx.Writer.WriteHeader(http.StatusNoContent)
		return
	}

	// WHIP/WHEP, outside session
	if m := reWHIPWHEPNoID.FindStringSubmatch(ctx.Request.URL.Path); m != nil {
		switch ctx.Request.Method {
		case http.MethodOptions:
			s.onWHIPOptions(ctx, m[1], m[2] == "whip")

		case http.MethodPost:
			s.onWHIPPost(ctx, m[1], m[2] == "whip")

		case http.MethodGet, http.MethodHead, http.MethodPut:
			// RFC draft-ietf-whip-09
			// The WHIP endpoints MUST return an "405 Method Not Allowed" response
			// for any HTTP GET, HEAD or PUT requests
			webrtcWriteError(ctx, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		}
		return
	}

	// WHIP/WHEP, inside session
	if m := reWHIPWHEPWithID.FindStringSubmatch(ctx.Request.URL.Path); m != nil {
		switch ctx.Request.Method {
		case http.MethodPatch:
			s.onWHIPPatch(ctx, m[3])

		case http.MethodDelete:
			s.onWHIPDelete(ctx, m[3])
		}
		return
	}

	// static resources
	if ctx.Request.Method == http.MethodGet {
		switch {
		case ctx.Request.URL.Path == "/favicon.ico":

		case len(ctx.Request.URL.Path) >= 2:
			switch {
			case len(ctx.Request.URL.Path) > len("/publish") && strings.HasSuffix(ctx.Request.URL.Path, "/publish"):
				s.onPage(ctx, ctx.Request.URL.Path[1:len(ctx.Request.URL.Path)-len("/publish")], true)

			case ctx.Request.URL.Path[len(ctx.Request.URL.Path)-1] != '/':
				ctx.Writer.Header().Set("Location", ctx.Request.URL.Path[1:]+"/")
				ctx.Writer.WriteHeader(http.StatusMovedPermanently)

			default:
				s.onPage(ctx, ctx.Request.URL.Path[1:len(ctx.Request.URL.Path)-1], false)
			}
		}
		return
	}
}
