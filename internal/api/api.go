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
	return reflect.ValueOf(i).Kind() != reflect.Ptr || reflect.ValueOf(i).IsNil()
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

	ret.Segments = make([]*defs.APIRecordingSegment, len(segments))

	for i, seg := range segments {
		ret.Segments[i] = &defs.APIRecordingSegment{
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
	ctx.Status(http.StatusOK)
}

func (a *API) onPathsList(ctx *gin.Context) {
	data, err := a.PathManager.APIPathsList()
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	// Filter by search parameter if provided
	search := ctx.Query("search")
	if search != "" {
		filteredItems := make([]*defs.APIPath, 0, len(data.Items))
		for _, item := range data.Items {
			if strings.Contains(strings.ToLower(item.Name), strings.ToLower(search)) {
				filteredItems = append(filteredItems, item)
			}
		}
		data.Items = filteredItems
	}

	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onPathsGet(ctx *gin.Context) {
	pathName, ok := paramName(ctx)
	if !ok {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid name"))
		return
	}

	data, err := a.PathManager.APIPathsGet(pathName)
	if err != nil {
		if errors.Is(err, conf.ErrPathNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onRTSPConnsList(ctx *gin.Context) {
	data, err := a.RTSPServer.APIConnsList()
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onRTSPConnsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	data, err := a.RTSPServer.APIConnsGet(uuid)
	if err != nil {
		if errors.Is(err, rtsp.ErrConnNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onRTSPSessionsList(ctx *gin.Context) {
	data, err := a.RTSPServer.APISessionsList()
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onRTSPSessionsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	data, err := a.RTSPServer.APISessionsGet(uuid)
	if err != nil {
		if errors.Is(err, rtsp.ErrSessionNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onRTSPSessionsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.RTSPServer.APISessionsKick(uuid)
	if err != nil {
		if errors.Is(err, rtsp.ErrSessionNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *API) onRTSPSConnsList(ctx *gin.Context) {
	data, err := a.RTSPSServer.APIConnsList()
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onRTSPSConnsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	data, err := a.RTSPSServer.APIConnsGet(uuid)
	if err != nil {
		if errors.Is(err, rtsp.ErrConnNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onRTSPSSessionsList(ctx *gin.Context) {
	data, err := a.RTSPSServer.APISessionsList()
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onRTSPSSessionsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	data, err := a.RTSPSServer.APISessionsGet(uuid)
	if err != nil {
		if errors.Is(err, rtsp.ErrSessionNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onRTSPSSessionsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.RTSPSServer.APISessionsKick(uuid)
	if err != nil {
		if errors.Is(err, rtsp.ErrSessionNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *API) onRTMPConnsList(ctx *gin.Context) {
	data, err := a.RTMPServer.APIConnsList()
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onRTMPConnsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	data, err := a.RTMPServer.APIConnsGet(uuid)
	if err != nil {
		if errors.Is(err, rtmp.ErrConnNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onRTMPConnsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.RTMPServer.APIConnsKick(uuid)
	if err != nil {
		if errors.Is(err, rtmp.ErrConnNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *API) onRTMPSConnsList(ctx *gin.Context) {
	data, err := a.RTMPSServer.APIConnsList()
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onRTMPSConnsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	data, err := a.RTMPSServer.APIConnsGet(uuid)
	if err != nil {
		if errors.Is(err, rtmp.ErrConnNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onRTMPSConnsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.RTMPSServer.APIConnsKick(uuid)
	if err != nil {
		if errors.Is(err, rtmp.ErrConnNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *API) onHLSMuxersList(ctx *gin.Context) {
	data, err := a.HLSServer.APIMuxersList()
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onHLSMuxersGet(ctx *gin.Context) {
	pathName, ok := paramName(ctx)
	if !ok {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid name"))
		return
	}

	data, err := a.HLSServer.APIMuxersGet(pathName)
	if err != nil {
		if errors.Is(err, hls.ErrMuxerNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onWebRTCSessionsList(ctx *gin.Context) {
	data, err := a.WebRTCServer.APISessionsList()
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onWebRTCSessionsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	data, err := a.WebRTCServer.APISessionsGet(uuid)
	if err != nil {
		if errors.Is(err, webrtc.ErrSessionNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onWebRTCSessionsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.WebRTCServer.APISessionsKick(uuid)
	if err != nil {
		if errors.Is(err, webrtc.ErrSessionNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *API) onSRTConnsList(ctx *gin.Context) {
	data, err := a.SRTServer.APIConnsList()
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onSRTConnsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	data, err := a.SRTServer.APIConnsGet(uuid)
	if err != nil {
		if errors.Is(err, srt.ErrConnNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onSRTConnsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.SRTServer.APIConnsKick(uuid)
	if err != nil {
		if errors.Is(err, srt.ErrConnNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *API) onRecordingsList(ctx *gin.Context) {
	a.mutex.RLock()
	c := a.Conf
	a.mutex.RUnlock()

	pathNames := recordstore.FindAllPathsWithSegments(c.Paths)

	data := defs.APIRecordingList{}

	data.ItemCount = len(pathNames)
	pageCount, err := paginate(&pathNames, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}
	data.PageCount = pageCount

	data.Items = make([]*defs.APIRecording, len(pathNames))

	for i, pathName := range pathNames {
		pathConf, _, _ := conf.FindPathConf(c.Paths, pathName)
		data.Items[i] = recordingsOfPath(pathConf, pathName)
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onRecordingsGet(ctx *gin.Context) {
	pathName, ok := paramName(ctx)
	if !ok {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid name"))
		return
	}

	a.mutex.RLock()
	c := a.Conf
	a.mutex.RUnlock()

	pathConf, _, err := conf.FindPathConf(c.Paths, pathName)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	ctx.JSON(http.StatusOK, recordingsOfPath(pathConf, pathName))
}

func (a *API) onRecordingDeleteSegment(ctx *gin.Context) {
	pathName := ctx.Query("path")

	start, err := time.Parse(time.RFC3339, ctx.Query("start"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid 'start' parameter: %w", err))
		return
	}

	a.mutex.RLock()
	c := a.Conf
	a.mutex.RUnlock()

	pathConf, _, err := conf.FindPathConf(c.Paths, pathName)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	pathFormat := recordstore.PathAddExtension(
		strings.ReplaceAll(pathConf.RecordPath, "%path", pathName),
		pathConf.RecordFormat,
	)

	segmentPath := recordstore.Path{
		Start: start,
	}.Encode(pathFormat)

	err = os.Remove(segmentPath)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	ctx.Status(http.StatusOK)
	a.writeOK(ctx)
}

// ReloadConf is called by core.
func (a *API) ReloadConf(conf *conf.Conf) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.Conf = conf
}
