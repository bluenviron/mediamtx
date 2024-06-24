package webrtc

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
	"github.com/bluenviron/mediamtx/internal/protocols/webrtc"
	"github.com/bluenviron/mediamtx/internal/restrictnetwork"
)

//go:embed publish_index.html
var publishIndex []byte

//go:embed read_index.html
var readIndex []byte

var (
	reWHIPWHEPNoID   = regexp.MustCompile("^/(.+?)/(whip|whep)$")
	reWHIPWHEPWithID = regexp.MustCompile("^/(.+?)/(whip|whep)/(.+?)$")
)

func mergePathAndQuery(path string, rawQuery string) string {
	res := path
	if rawQuery != "" {
		res += "?" + rawQuery
	}
	return res
}

func writeError(ctx *gin.Context, statusCode int, err error) {
	ctx.JSON(statusCode, &defs.APIError{
		Error: err.Error(),
	})
}

func sessionLocation(publish bool, path string, secret uuid.UUID) string {
	ret := "/" + path + "/"
	if publish {
		ret += "whip"
	} else {
		ret += "whep"
	}
	ret += "/" + secret.String()
	return ret
}

func addJWTFromAuthorization(rawQuery string, auth string) string {
	jwt := strings.TrimPrefix(auth, "Bearer ")
	if rawQuery != "" {
		if v, err := url.ParseQuery(rawQuery); err == nil && v.Get("jwt") == "" {
			v.Set("jwt", jwt)
			return v.Encode()
		}
	}
	return url.Values{"jwt": []string{jwt}}.Encode()
}

type httpServer struct {
	address        string
	encryption     bool
	serverKey      string
	serverCert     string
	allowOrigin    string
	trustedProxies conf.IPNetworks
	readTimeout    conf.StringDuration
	pathManager    serverPathManager
	parent         *Server

	inner *httpp.WrappedServer
}

func (s *httpServer) initialize() error {
	router := gin.New()
	router.SetTrustedProxies(s.trustedProxies.ToTrustedProxies()) //nolint:errcheck
	router.NoRoute(s.onRequest)

	network, address := restrictnetwork.Restrict("tcp", s.address)

	s.inner = &httpp.WrappedServer{
		Network:     network,
		Address:     address,
		ReadTimeout: time.Duration(s.readTimeout),
		Encryption:  s.encryption,
		ServerCert:  s.serverCert,
		ServerKey:   s.serverKey,
		Handler:     router,
		Parent:      s,
	}
	err := s.inner.Initialize()
	if err != nil {
		return err
	}

	return nil
}

// Log implements logger.Writer.
func (s *httpServer) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, format, args...)
}

func (s *httpServer) close() {
	s.inner.Close()
}

func (s *httpServer) checkAuthOutsideSession(ctx *gin.Context, pathName string, publish bool) bool {
	user, pass, hasCredentials := ctx.Request.BasicAuth()
	q := ctx.Request.URL.RawQuery

	if h := ctx.Request.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		// JWT in authorization bearer -> JWT in query parameters
		q = addJWTFromAuthorization(q, h)

		// credentials in authorization bearer -> credentials in authorization basic
		if parts := strings.Split(strings.TrimPrefix(h, "Bearer "), ":"); len(parts) == 2 {
			user = parts[0]
			pass = parts[1]
		}
	}

	_, err := s.pathManager.FindPathConf(defs.PathFindPathConfReq{
		AccessRequest: defs.PathAccessRequest{
			Name:    pathName,
			Query:   q,
			Publish: publish,
			IP:      net.ParseIP(ctx.ClientIP()),
			User:    user,
			Pass:    pass,
			Proto:   auth.ProtocolWebRTC,
		},
	})
	if err != nil {
		var terr auth.Error
		if errors.As(err, &terr) {
			if !hasCredentials {
				ctx.Header("WWW-Authenticate", `Basic realm="mediamtx"`)
				ctx.Writer.WriteHeader(http.StatusUnauthorized)
				return false
			}

			s.Log(logger.Info, "connection %v failed to authenticate: %v", httpp.RemoteAddr(ctx), terr.Message)

			// wait some seconds to mitigate brute force attacks
			<-time.After(auth.PauseAfterError)

			writeError(ctx, http.StatusUnauthorized, terr)
			return false
		}

		writeError(ctx, http.StatusInternalServerError, err)
		return false
	}

	return true
}

func (s *httpServer) onWHIPOptions(ctx *gin.Context, pathName string, publish bool) {
	if !s.checkAuthOutsideSession(ctx, pathName, publish) {
		return
	}

	servers, err := s.parent.generateICEServers(true)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.Writer.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST, PATCH, DELETE")
	ctx.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match")
	ctx.Writer.Header().Set("Access-Control-Expose-Headers", "Link")
	ctx.Writer.Header()["Link"] = webrtc.LinkHeaderMarshal(servers)
	ctx.Writer.WriteHeader(http.StatusNoContent)
}

func (s *httpServer) onWHIPPost(ctx *gin.Context, pathName string, publish bool) {
	contentType := httpp.ParseContentType(ctx.Request.Header.Get("Content-Type"))
	if contentType != "application/sdp" {
		writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid Content-Type"))
		return
	}

	offer, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		return
	}

	user, pass, _ := ctx.Request.BasicAuth()
	q := ctx.Request.URL.RawQuery

	if h := ctx.Request.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		// JWT in authorization bearer -> JWT in query parameters
		q = addJWTFromAuthorization(q, h)

		// credentials in authorization bearer -> credentials in authorization basic
		if parts := strings.Split(strings.TrimPrefix(h, "Bearer "), ":"); len(parts) == 2 {
			user = parts[0]
			pass = parts[1]
		}
	}

	res := s.parent.newSession(webRTCNewSessionReq{
		pathName:   pathName,
		remoteAddr: httpp.RemoteAddr(ctx),
		query:      q,
		user:       user,
		pass:       pass,
		offer:      offer,
		publish:    publish,
	})
	if res.err != nil {
		writeError(ctx, res.errStatusCode, res.err)
		return
	}

	servers, err := s.parent.generateICEServers(true)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.Writer.Header().Set("Content-Type", "application/sdp")
	ctx.Writer.Header().Set("Access-Control-Expose-Headers", "ETag, ID, Accept-Patch, Link, Location")
	ctx.Writer.Header().Set("ETag", "*")
	ctx.Writer.Header().Set("ID", res.sx.uuid.String())
	ctx.Writer.Header().Set("Accept-Patch", "application/trickle-ice-sdpfrag")
	ctx.Writer.Header()["Link"] = webrtc.LinkHeaderMarshal(servers)
	ctx.Writer.Header().Set("Location", sessionLocation(publish, pathName, res.sx.secret))
	ctx.Writer.WriteHeader(http.StatusCreated)
	ctx.Writer.Write(res.answer)
}

func (s *httpServer) onWHIPPatch(ctx *gin.Context, pathName string, rawSecret string) {
	secret, err := uuid.Parse(rawSecret)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid secret"))
		return
	}

	contentType := httpp.ParseContentType(ctx.Request.Header.Get("Content-Type"))
	if contentType != "application/trickle-ice-sdpfrag" {
		writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid Content-Type"))
		return
	}

	byts, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		return
	}

	candidates, err := webrtc.ICEFragmentUnmarshal(byts)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err)
		return
	}

	res := s.parent.addSessionCandidates(webRTCAddSessionCandidatesReq{
		pathName:   pathName,
		secret:     secret,
		candidates: candidates,
	})
	if res.err != nil {
		if errors.Is(res.err, ErrSessionNotFound) {
			writeError(ctx, http.StatusNotFound, res.err)
		} else {
			writeError(ctx, http.StatusInternalServerError, res.err)
		}
		return
	}

	ctx.Writer.WriteHeader(http.StatusNoContent)
}

func (s *httpServer) onWHIPDelete(ctx *gin.Context, pathName string, rawSecret string) {
	secret, err := uuid.Parse(rawSecret)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid secret"))
		return
	}

	err = s.parent.deleteSession(webRTCDeleteSessionReq{
		pathName: pathName,
		secret:   secret,
	})
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			writeError(ctx, http.StatusNotFound, err)
		} else {
			writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.Writer.WriteHeader(http.StatusOK)
}

func (s *httpServer) onPage(ctx *gin.Context, pathName string, publish bool) {
	if !s.checkAuthOutsideSession(ctx, pathName, publish) {
		return
	}

	ctx.Writer.Header().Set("Cache-Control", "max-age=3600")
	ctx.Writer.Header().Set("Content-Type", "text/html")
	ctx.Writer.WriteHeader(http.StatusOK)

	if publish {
		ctx.Writer.Write(publishIndex)
	} else {
		ctx.Writer.Write(readIndex)
	}
}

func (s *httpServer) onRequest(ctx *gin.Context) {
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
			writeError(ctx, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		}
		return
	}

	// WHIP/WHEP, inside session
	if m := reWHIPWHEPWithID.FindStringSubmatch(ctx.Request.URL.Path); m != nil {
		switch ctx.Request.Method {
		case http.MethodPatch:
			s.onWHIPPatch(ctx, m[1], m[3])

		case http.MethodDelete:
			s.onWHIPDelete(ctx, m[1], m[3])
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
				ctx.Writer.Header().Set("Location", mergePathAndQuery(ctx.Request.URL.Path+"/", ctx.Request.URL.RawQuery))
				ctx.Writer.WriteHeader(http.StatusMovedPermanently)

			default:
				s.onPage(ctx, ctx.Request.URL.Path[1:len(ctx.Request.URL.Path)-1], false)
			}
		}
		return
	}
}
