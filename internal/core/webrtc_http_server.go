package core

import (
	_ "embed"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pion/webrtc/v3"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/httpserv"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/whip"
)

//go:embed webrtc_publish_index.html
var webrtcPublishIndex []byte

//go:embed webrtc_read_index.html
var webrtcReadIndex []byte

type webRTCHTTPServerParent interface {
	logger.Writer
	generateICEServers() ([]webrtc.ICEServer, error)
	newSession(req webRTCNewSessionReq) webRTCNewSessionRes
	addSessionCandidates(req webRTCAddSessionCandidatesReq) webRTCAddSessionCandidatesRes
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

	network, address := restrictNetwork("tcp", address)

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
			ctx.Writer.WriteHeader(http.StatusNoContent)
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
	remoteAddr := net.JoinHostPort(ip, port)
	user, pass, hasCredentials := ctx.Request.BasicAuth()

	// if request doesn't belong to a session, check authentication here
	if !isWHIPorWHEP || ctx.Request.Method == http.MethodOptions {
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
			if terr, ok := res.err.(*errAuthentication); ok {
				if !hasCredentials {
					ctx.Header("WWW-Authenticate", `Basic realm="mediamtx"`)
					ctx.Writer.WriteHeader(http.StatusUnauthorized)
					return
				}

				s.Log(logger.Info, "connection %v failed to authenticate: %v", remoteAddr, terr.message)

				// wait some seconds to stop brute force attacks
				<-time.After(webrtcPauseAfterAuthError)

				ctx.Writer.WriteHeader(http.StatusUnauthorized)
				return
			}

			ctx.Writer.WriteHeader(http.StatusNotFound)
			return
		}
	}

	switch fname {
	case "":
		ctx.Writer.Header().Set("Cache-Control", "max-age=3600")
		ctx.Writer.Header().Set("Content-Type", "text/html")
		ctx.Writer.WriteHeader(http.StatusOK)
		ctx.Writer.Write(webrtcReadIndex)

	case "publish":
		ctx.Writer.Header().Set("Cache-Control", "max-age=3600")
		ctx.Writer.Header().Set("Content-Type", "text/html")
		ctx.Writer.WriteHeader(http.StatusOK)
		ctx.Writer.Write(webrtcPublishIndex)

	case "whip", "whep":
		switch ctx.Request.Method {
		case http.MethodOptions:
			servers, err := s.parent.generateICEServers()
			if err != nil {
				ctx.Writer.WriteHeader(http.StatusInternalServerError)
				return
			}

			ctx.Writer.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST, PATCH")
			ctx.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match")
			ctx.Writer.Header()["Link"] = whip.LinkHeaderMarshal(servers)
			ctx.Writer.WriteHeader(http.StatusNoContent)

		case http.MethodPost:
			if ctx.Request.Header.Get("Content-Type") != "application/sdp" {
				ctx.Writer.WriteHeader(http.StatusBadRequest)
				return
			}

			offer, err := io.ReadAll(ctx.Request.Body)
			if err != nil {
				return
			}

			res := s.parent.newSession(webRTCNewSessionReq{
				pathName:   dir,
				remoteAddr: remoteAddr,
				query:      ctx.Request.URL.RawQuery,
				user:       user,
				pass:       pass,
				offer:      offer,
				publish:    (fname == "whip"),
			})
			if res.err != nil {
				ctx.Writer.WriteHeader(res.errStatusCode)
				return
			}

			servers, err := s.parent.generateICEServers()
			if err != nil {
				ctx.Writer.WriteHeader(http.StatusInternalServerError)
				return
			}

			ctx.Writer.Header().Set("Content-Type", "application/sdp")
			ctx.Writer.Header().Set("Access-Control-Expose-Headers", "E-Tag, Accept-Patch, Link")
			ctx.Writer.Header().Set("E-Tag", res.sx.secret.String())
			ctx.Writer.Header().Set("ID", res.sx.uuid.String())
			ctx.Writer.Header().Set("Accept-Patch", "application/trickle-ice-sdpfrag")
			ctx.Writer.Header()["Link"] = whip.LinkHeaderMarshal(servers)
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

			candidates, err := whip.ICEFragmentUnmarshal(byts)
			if err != nil {
				ctx.Writer.WriteHeader(http.StatusBadRequest)
				return
			}

			res := s.parent.addSessionCandidates(webRTCAddSessionCandidatesReq{
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
