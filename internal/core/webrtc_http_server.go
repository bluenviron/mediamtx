package core

import (
	_ "embed"
	"fmt"
	"io"
	"net"
	"net/http"
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
	genICEServers() []webrtc.ICEServer
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

	if !strings.HasSuffix(pa, "/whip") && !strings.HasSuffix(pa, "/whep") {
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

	case "publish":
		ctx.Writer.Header().Set("Content-Type", "text/html")
		ctx.Writer.WriteHeader(http.StatusOK)
		ctx.Writer.Write(webrtcPublishIndex)

	case "whip", "whep":
		switch ctx.Request.Method {
		case http.MethodOptions:
			ctx.Writer.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			ctx.Writer.Header().Set("Access-Control-Allow-Headers", ctx.Request.Header.Get("Access-Control-Request-Headers"))
			ctx.Writer.Header()["Link"] = iceServersToLinkHeader(s.parent.genICEServers())
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
				pathName:     dir,
				remoteAddr:   ctx.ClientIP(),
				offer:        offer,
				publish:      (fname == "whip"),
				videoCodec:   ctx.Query("video_codec"),
				audioCodec:   ctx.Query("audio_codec"),
				videoBitrate: ctx.Query("video_bitrate"),
			})
			if res.err != nil {
				if res.errStatusCode != 0 {
					ctx.Writer.WriteHeader(res.errStatusCode)
				}
				return
			}

			ctx.Writer.Header().Set("Content-Type", "application/sdp")
			ctx.Writer.Header().Set("Access-Control-Expose-Headers", "E-Tag", "Accept-Patch", "Link")
			ctx.Writer.Header().Set("E-Tag", res.sx.secret.String())
			ctx.Writer.Header().Set("Accept-Patch", "application/trickle-ice-sdpfrag")
			ctx.Writer.Header()["Link"] = iceServersToLinkHeader(s.parent.genICEServers())
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
