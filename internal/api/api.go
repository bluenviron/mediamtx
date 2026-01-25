// Package api contains the API server.
package api //nolint:revive

import (
	"net"
	"net/http"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
	"github.com/bluenviron/mediamtx/internal/recordstore"
)

func interfaceIsEmpty(i any) bool {
	return reflect.ValueOf(i).Kind() != reflect.Pointer || reflect.ValueOf(i).IsNil()
}

func sortedKeys(paths map[string]*conf.Path) []string {
	ret := make([]string, len(paths))
	i := 0
	for name := range paths {
		ret[i] = name
		i++
	}
	sort.Strings(ret)
	return ret
}

func paramName(ctx *gin.Context) (string, bool) {
	name := ctx.Param("name")

	if len(name) < 2 || name[0] != '/' {
		return "", false
	}

	return name[1:], true
}

func recordingsOfPath(
	pathConf *conf.Path,
	pathName string,
) *defs.APIRecording {
	ret := &defs.APIRecording{
		Name: pathName,
	}

	segments, _ := recordstore.FindSegments(pathConf, pathName, nil, nil)

	ret.Segments = make([]defs.APIRecordingSegment, len(segments))

	for i, seg := range segments {
		ret.Segments[i] = defs.APIRecordingSegment{
			Start: seg.Start,
		}
	}

	return ret
}

type apiAuthManager interface {
	Authenticate(req *auth.Request) *auth.Error
	RefreshJWTJWKS()
}

type apiParent interface {
	logger.Writer
	APIConfigSet(conf *conf.Conf)
}

// API is an API server.
type API struct {
	Version        string
	Started        time.Time
	Address        string
	Encryption     bool
	ServerKey      string
	ServerCert     string
	AllowOrigins   []string
	TrustedProxies conf.IPNetworks
	ReadTimeout    conf.Duration
	WriteTimeout   conf.Duration
	Conf           *conf.Conf
	AuthManager    apiAuthManager
	PathManager    defs.APIPathManager
	RTSPServer     defs.APIRTSPServer
	RTSPSServer    defs.APIRTSPServer
	RTMPServer     defs.APIRTMPServer
	RTMPSServer    defs.APIRTMPServer
	HLSServer      defs.APIHLSServer
	WebRTCServer   defs.APIWebRTCServer
	SRTServer      defs.APISRTServer
	Parent         apiParent

	httpServer *httpp.Server
	mutex      sync.RWMutex
}

// Initialize initializes API.
func (a *API) Initialize() error {
	router := gin.New()
	router.SetTrustedProxies(a.TrustedProxies.ToTrustedProxies()) //nolint:errcheck

	router.Use(a.middlewarePreflightRequests)
	router.Use(a.middlewareAuth)

	group := router.Group("/v3")

	group.GET("/info", a.onInfo)

	group.POST("/auth/jwks/refresh", a.onAuthJwksRefresh)

	group.GET("/config/global/get", a.onConfigGlobalGet)
	group.PATCH("/config/global/patch", a.onConfigGlobalPatch)

	group.GET("/config/pathdefaults/get", a.onConfigPathDefaultsGet)
	group.PATCH("/config/pathdefaults/patch", a.onConfigPathDefaultsPatch)

	group.GET("/config/paths/list", a.onConfigPathsList)
	group.GET("/config/paths/get/*name", a.onConfigPathsGet)
	group.POST("/config/paths/add/*name", a.onConfigPathsAdd)
	group.PATCH("/config/paths/patch/*name", a.onConfigPathsPatch)
	group.POST("/config/paths/replace/*name", a.onConfigPathsReplace)
	group.DELETE("/config/paths/delete/*name", a.onConfigPathsDelete)

	group.GET("/paths/list", a.onPathsList)
	group.GET("/paths/get/*name", a.onPathsGet)

	if !interfaceIsEmpty(a.HLSServer) {
		group.GET("/hlsmuxers/list", a.onHLSMuxersList)
		group.GET("/hlsmuxers/get/*name", a.onHLSMuxersGet)
	}

	if !interfaceIsEmpty(a.RTSPServer) {
		group.GET("/rtspconns/list", a.onRTSPConnsList)
		group.GET("/rtspconns/get/:id", a.onRTSPConnsGet)
		group.GET("/rtspsessions/list", a.onRTSPSessionsList)
		group.GET("/rtspsessions/get/:id", a.onRTSPSessionsGet)
		group.POST("/rtspsessions/kick/:id", a.onRTSPSessionsKick)
	}

	if !interfaceIsEmpty(a.RTSPSServer) {
		group.GET("/rtspsconns/list", a.onRTSPSConnsList)
		group.GET("/rtspsconns/get/:id", a.onRTSPSConnsGet)
		group.GET("/rtspssessions/list", a.onRTSPSSessionsList)
		group.GET("/rtspssessions/get/:id", a.onRTSPSSessionsGet)
		group.POST("/rtspssessions/kick/:id", a.onRTSPSSessionsKick)
	}

	if !interfaceIsEmpty(a.RTMPServer) {
		group.GET("/rtmpconns/list", a.onRTMPConnsList)
		group.GET("/rtmpconns/get/:id", a.onRTMPConnsGet)
		group.POST("/rtmpconns/kick/:id", a.onRTMPConnsKick)
	}

	if !interfaceIsEmpty(a.RTMPSServer) {
		group.GET("/rtmpsconns/list", a.onRTMPSConnsList)
		group.GET("/rtmpsconns/get/:id", a.onRTMPSConnsGet)
		group.POST("/rtmpsconns/kick/:id", a.onRTMPSConnsKick)
	}

	if !interfaceIsEmpty(a.WebRTCServer) {
		group.GET("/webrtcsessions/list", a.onWebRTCSessionsList)
		group.GET("/webrtcsessions/get/:id", a.onWebRTCSessionsGet)
		group.POST("/webrtcsessions/kick/:id", a.onWebRTCSessionsKick)
	}

	if !interfaceIsEmpty(a.SRTServer) {
		group.GET("/srtconns/list", a.onSRTConnsList)
		group.GET("/srtconns/get/:id", a.onSRTConnsGet)
		group.POST("/srtconns/kick/:id", a.onSRTConnsKick)
	}

	group.GET("/recordings/list", a.onRecordingsList)
	group.GET("/recordings/get/*name", a.onRecordingsGet)
	group.DELETE("/recordings/deletesegment", a.onRecordingDeleteSegment)

	a.httpServer = &httpp.Server{
		Address:      a.Address,
		AllowOrigins: a.AllowOrigins,
		ReadTimeout:  time.Duration(a.ReadTimeout),
		WriteTimeout: time.Duration(a.WriteTimeout),
		Encryption:   a.Encryption,
		ServerCert:   a.ServerCert,
		ServerKey:    a.ServerKey,
		Handler:      router,
		Parent:       a,
	}
	err := a.httpServer.Initialize()
	if err != nil {
		return err
	}

	a.Log(logger.Info, "listener opened on "+a.Address)

	return nil
}

// Close closes the API.
func (a *API) Close() {
	a.Log(logger.Info, "listener is closing")
	a.httpServer.Close()
}

// Log implements logger.Writer.
func (a *API) Log(level logger.Level, format string, args ...any) {
	a.Parent.Log(level, "[API] "+format, args...)
}

func (a *API) writeError(ctx *gin.Context, status int, err error) {
	// show error in logs
	a.Log(logger.Error, err.Error())

	// add error to response
	ctx.JSON(status, &defs.APIError{
		Status: "error",
		Error:  err.Error(),
	})
}

func (a *API) writeOK(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, &defs.APIOK{Status: "ok"})
}

func (a *API) middlewarePreflightRequests(ctx *gin.Context) {
	if ctx.Request.Method == http.MethodOptions &&
		ctx.Request.Header.Get("Access-Control-Request-Method") != "" {
		ctx.Header("Access-Control-Allow-Methods", "OPTIONS, GET, POST, PATCH, DELETE")
		ctx.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		ctx.AbortWithStatus(http.StatusNoContent)
		return
	}
}

func (a *API) middlewareAuth(ctx *gin.Context) {
	req := &auth.Request{
		Action:      conf.AuthActionAPI,
		Query:       ctx.Request.URL.RawQuery,
		Credentials: httpp.Credentials(ctx.Request),
		IP:          net.ParseIP(ctx.ClientIP()),
	}

	err := a.AuthManager.Authenticate(req)
	if err != nil {
		if err.AskCredentials {
			ctx.Header("WWW-Authenticate", `Basic realm="mediamtx"`)
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, &defs.APIError{
				Status: "error",
				Error:  "authentication error",
			})
			return
		}

		a.Log(logger.Info, "connection %v failed to authenticate: %v", httpp.RemoteAddr(ctx), err.Wrapped)

		// wait some seconds to delay brute force attacks
		<-time.After(auth.PauseAfterError)

		ctx.AbortWithStatusJSON(http.StatusUnauthorized, &defs.APIError{
			Status: "error",
			Error:  "authentication error",
		})
		return
	}
}

func (a *API) onInfo(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, &defs.APIInfo{
		Version: a.Version,
		Started: a.Started,
	})
}

func (a *API) onAuthJwksRefresh(ctx *gin.Context) {
	a.AuthManager.RefreshJWTJWKS()
	a.writeOK(ctx)
}

// ReloadConf is called by core.
func (a *API) ReloadConf(conf *conf.Conf) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.Conf = conf
}
