package core

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httputil"
	"reflect"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

func interfaceIsEmpty(i interface{}) bool {
	return reflect.ValueOf(i).Kind() != reflect.Ptr || reflect.ValueOf(i).IsNil()
}

func fillStruct(dest interface{}, source interface{}) {
	rvsource := reflect.ValueOf(source)
	rvdest := reflect.ValueOf(dest)
	nf := rvsource.NumField()
	for i := 0; i < nf; i++ {
		fnew := rvsource.Field(i)
		if !fnew.IsNil() {
			f := rvdest.Elem().FieldByName(rvsource.Type().Field(i).Name)
			if f.Kind() == reflect.Ptr {
				f.Set(fnew)
			} else {
				f.Set(fnew.Elem())
			}
		}
	}
}

func cloneStruct(dest interface{}, source interface{}) {
	enc, _ := json.Marshal(source)
	_ = json.Unmarshal(enc, dest)
}

func loadConfData(ctx *gin.Context) (interface{}, error) {
	var in struct {
		// general
		LogLevel                  *conf.LogLevel        `json:"logLevel"`
		LogDestinations           *conf.LogDestinations `json:"logDestinations"`
		LogFile                   *string               `json:"logFile"`
		ReadTimeout               *conf.StringDuration  `json:"readTimeout"`
		WriteTimeout              *conf.StringDuration  `json:"writeTimeout"`
		ReadBufferCount           *int                  `json:"readBufferCount"`
		ExternalAuthenticationURL *string               `json:"externalAuthenticationURL"`
		API                       *bool                 `json:"api"`
		APIAddress                *string               `json:"apiAddress"`
		Metrics                   *bool                 `json:"metrics"`
		MetricsAddress            *string               `json:"metricsAddress"`
		PPROF                     *bool                 `json:"pprof"`
		PPROFAddress              *string               `json:"pprofAddress"`
		RunOnConnect              *string               `json:"runOnConnect"`
		RunOnConnectRestart       *bool                 `json:"runOnConnectRestart"`

		// RTSP
		RTSPDisable       *bool             `json:"rtspDisable"`
		Protocols         *conf.Protocols   `json:"protocols"`
		Encryption        *conf.Encryption  `json:"encryption"`
		RTSPAddress       *string           `json:"rtspAddress"`
		RTSPSAddress      *string           `json:"rtspsAddress"`
		RTPAddress        *string           `json:"rtpAddress"`
		RTCPAddress       *string           `json:"rtcpAddress"`
		MulticastIPRange  *string           `json:"multicastIPRange"`
		MulticastRTPPort  *int              `json:"multicastRTPPort"`
		MulticastRTCPPort *int              `json:"multicastRTCPPort"`
		ServerKey         *string           `json:"serverKey"`
		ServerCert        *string           `json:"serverCert"`
		AuthMethods       *conf.AuthMethods `json:"authMethods"`

		// RTMP
		RTMPDisable *bool   `json:"rtmpDisable"`
		RTMPAddress *string `json:"rtmpAddress"`

		// HLS
		HLSDisable         *bool                `json:"hlsDisable"`
		HLSAddress         *string              `json:"hlsAddress"`
		HLSAlwaysRemux     *bool                `json:"hlsAlwaysRemux"`
		HLSVariant         *conf.HLSVariant     `json:"hlsVariant"`
		HLSSegmentCount    *int                 `json:"hlsSegmentCount"`
		HLSSegmentDuration *conf.StringDuration `json:"hlsSegmentDuration"`
		HLSPartDuration    *conf.StringDuration `json:"hlsPartDuration"`
		HLSSegmentMaxSize  *conf.StringSize     `json:"hlsSegmentMaxSize"`
		HLSAllowOrigin     *string              `json:"hlsAllowOrigin"`
		HLSEncryption      *bool                `json:"hlsEncryption"`
		HLSServerKey       *string              `json:"hlsServerKey"`
		HLSServerCert      *string              `json:"hlsServerCert"`
		HLSTrustedProxies  *conf.IPsOrCIDRs     `json:"hlsTrustedProxies"`
	}
	err := json.NewDecoder(ctx.Request.Body).Decode(&in)
	if err != nil {
		return nil, err
	}

	return in, err
}

func loadConfPathData(ctx *gin.Context) (interface{}, error) {
	var in struct {
		// source
		Source                     *string              `json:"source"`
		SourceProtocol             *conf.SourceProtocol `json:"sourceProtocol"`
		SourceAnyPortEnable        *bool                `json:"sourceAnyPortEnable"`
		SourceFingerprint          *string              `json:"sourceFingerprint"`
		SourceOnDemand             *bool                `json:"sourceOnDemand"`
		SourceOnDemandStartTimeout *conf.StringDuration `json:"sourceOnDemandStartTimeout"`
		SourceOnDemandCloseAfter   *conf.StringDuration `json:"sourceOnDemandCloseAfter"`
		SourceRedirect             *string              `json:"sourceRedirect"`
		DisablePublisherOverride   *bool                `json:"disablePublisherOverride"`
		Fallback                   *string              `json:"fallback"`

		// authentication
		PublishUser *conf.Credential `json:"publishUser"`
		PublishPass *conf.Credential `json:"publishPass"`
		PublishIPs  *conf.IPsOrCIDRs `json:"publishIPs"`
		ReadUser    *conf.Credential `json:"readUser"`
		ReadPass    *conf.Credential `json:"readPass"`
		ReadIPs     *conf.IPsOrCIDRs `json:"readIPs"`

		// external commands
		RunOnInit               *string              `json:"runOnInit"`
		RunOnInitRestart        *bool                `json:"runOnInitRestart"`
		RunOnDemand             *string              `json:"runOnDemand"`
		RunOnDemandRestart      *bool                `json:"runOnDemandRestart"`
		RunOnDemandStartTimeout *conf.StringDuration `json:"runOnDemandStartTimeout"`
		RunOnDemandCloseAfter   *conf.StringDuration `json:"runOnDemandCloseAfter"`
		RunOnReady              *string              `json:"runOnReady"`
		RunOnReadyRestart       *bool                `json:"runOnReadyRestart"`
		RunOnRead               *string              `json:"runOnRead"`
		RunOnReadRestart        *bool                `json:"runOnReadRestart"`
	}
	err := json.NewDecoder(ctx.Request.Body).Decode(&in)
	if err != nil {
		return nil, err
	}

	return in, err
}

type apiPathManager interface {
	onAPIPathsList(req pathAPIPathsListReq) pathAPIPathsListRes
}

type apiRTSPServer interface {
	onAPISessionsList(req rtspServerAPISessionsListReq) rtspServerAPISessionsListRes
	onAPISessionsKick(req rtspServerAPISessionsKickReq) rtspServerAPISessionsKickRes
}

type apiRTMPServer interface {
	onAPIConnsList(req rtmpServerAPIConnsListReq) rtmpServerAPIConnsListRes
	onAPIConnsKick(req rtmpServerAPIConnsKickReq) rtmpServerAPIConnsKickRes
}

type apiHLSServer interface {
	onAPIHLSMuxersList(req hlsServerAPIMuxersListReq) hlsServerAPIMuxersListRes
}

type apiParent interface {
	Log(logger.Level, string, ...interface{})
	onAPIConfigSet(conf *conf.Conf)
}

type api struct {
	conf        *conf.Conf
	pathManager apiPathManager
	rtspServer  apiRTSPServer
	rtspsServer apiRTSPServer
	rtmpServer  apiRTMPServer
	hlsServer   apiHLSServer
	parent      apiParent

	mutex sync.Mutex
	s     *http.Server
}

func newAPI(
	address string,
	conf *conf.Conf,
	pathManager apiPathManager,
	rtspServer apiRTSPServer,
	rtspsServer apiRTSPServer,
	rtmpServer apiRTMPServer,
	hlsServer apiHLSServer,
	parent apiParent,
) (*api, error) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	a := &api{
		conf:        conf,
		pathManager: pathManager,
		rtspServer:  rtspServer,
		rtspsServer: rtspsServer,
		rtmpServer:  rtmpServer,
		hlsServer:   hlsServer,
		parent:      parent,
	}

	router := gin.New()
	router.SetTrustedProxies(nil)
	router.NoRoute(a.mwLog)
	group := router.Group("/", a.mwLog)

	group.GET("/v1/config/get", a.onConfigGet)
	group.POST("/v1/config/set", a.onConfigSet)
	group.POST("/v1/config/paths/add/*name", a.onConfigPathsAdd)
	group.POST("/v1/config/paths/edit/*name", a.onConfigPathsEdit)
	group.POST("/v1/config/paths/remove/*name", a.onConfigPathsDelete)

	group.GET("/v1/paths/list", a.onPathsList)

	if !interfaceIsEmpty(a.rtspServer) {
		group.GET("/v1/rtspsessions/list", a.onRTSPSessionsList)
		group.POST("/v1/rtspsessions/kick/:id", a.onRTSPSessionsKick)
	}

	if !interfaceIsEmpty(a.rtspsServer) {
		group.GET("/v1/rtspssessions/list", a.onRTSPSSessionsList)
		group.POST("/v1/rtspssessions/kick/:id", a.onRTSPSSessionsKick)
	}

	if !interfaceIsEmpty(a.rtmpServer) {
		group.GET("/v1/rtmpconns/list", a.onRTMPConnsList)
		group.POST("/v1/rtmpconns/kick/:id", a.onRTMPConnsKick)
	}

	if !interfaceIsEmpty(a.hlsServer) {
		group.GET("/v1/hlsmuxers/list", a.onHLSMuxersList)
	}

	a.s = &http.Server{Handler: router}

	go a.s.Serve(ln)

	a.log(logger.Info, "listener opened on "+address)

	return a, nil
}

func (a *api) close() {
	a.log(logger.Info, "listener is closing")
	a.s.Shutdown(context.Background())
}

func (a *api) log(level logger.Level, format string, args ...interface{}) {
	a.parent.Log(level, "[API] "+format, args...)
}

func (a *api) mwLog(ctx *gin.Context) {
	a.log(logger.Info, "[conn %v] %s %s", ctx.Request.RemoteAddr, ctx.Request.Method, ctx.Request.URL.Path)

	byts, _ := httputil.DumpRequest(ctx.Request, true)
	a.log(logger.Debug, "[conn %v] [c->s] %s", ctx.Request.RemoteAddr, string(byts))

	logw := &httpLogWriter{ResponseWriter: ctx.Writer}
	ctx.Writer = logw

	ctx.Writer.Header().Set("Server", "rtsp-simple-server")

	ctx.Next()

	a.log(logger.Debug, "[conn %v] [s->c] %s", ctx.Request.RemoteAddr, logw.dump())
}

func (a *api) onConfigGet(ctx *gin.Context) {
	a.mutex.Lock()
	c := a.conf
	a.mutex.Unlock()

	ctx.JSON(http.StatusOK, c)
}

func (a *api) onConfigSet(ctx *gin.Context) {
	in, err := loadConfData(ctx)
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	var newConf conf.Conf
	cloneStruct(&newConf, a.conf)
	fillStruct(&newConf, in)

	err = newConf.CheckAndFillMissing()
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	a.conf = &newConf

	// since reloading the configuration can cause the shutdown of the API,
	// call it in a goroutine
	go a.parent.onAPIConfigSet(&newConf)

	ctx.Status(http.StatusOK)
}

func (a *api) onConfigPathsAdd(ctx *gin.Context) {
	name := ctx.Param("name")
	if len(name) < 2 || name[0] != '/' {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	name = name[1:]

	in, err := loadConfPathData(ctx)
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	var newConf conf.Conf
	cloneStruct(&newConf, a.conf)

	if _, ok := newConf.Paths[name]; ok {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	newConfPath := &conf.PathConf{}
	fillStruct(newConfPath, in)

	newConf.Paths[name] = newConfPath

	err = newConf.CheckAndFillMissing()
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	a.conf = &newConf

	// since reloading the configuration can cause the shutdown of the API,
	// call it in a goroutine
	go a.parent.onAPIConfigSet(&newConf)

	ctx.Status(http.StatusOK)
}

func (a *api) onConfigPathsEdit(ctx *gin.Context) {
	name := ctx.Param("name")
	if len(name) < 2 || name[0] != '/' {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	name = name[1:]

	in, err := loadConfPathData(ctx)
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	var newConf conf.Conf
	cloneStruct(&newConf, a.conf)

	newConfPath, ok := newConf.Paths[name]
	if !ok {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	fillStruct(newConfPath, in)

	err = newConf.CheckAndFillMissing()
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	a.conf = &newConf

	// since reloading the configuration can cause the shutdown of the API,
	// call it in a goroutine
	go a.parent.onAPIConfigSet(&newConf)

	ctx.Status(http.StatusOK)
}

func (a *api) onConfigPathsDelete(ctx *gin.Context) {
	name := ctx.Param("name")
	if len(name) < 2 || name[0] != '/' {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	name = name[1:]

	a.mutex.Lock()
	defer a.mutex.Unlock()

	var newConf conf.Conf
	cloneStruct(&newConf, a.conf)

	if _, ok := newConf.Paths[name]; !ok {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	delete(newConf.Paths, name)

	err := newConf.CheckAndFillMissing()
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	a.conf = &newConf

	// since reloading the configuration can cause the shutdown of the API,
	// call it in a goroutine
	go a.parent.onAPIConfigSet(&newConf)

	ctx.Status(http.StatusOK)
}

func (a *api) onPathsList(ctx *gin.Context) {
	res := a.pathManager.onAPIPathsList(pathAPIPathsListReq{})
	if res.err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	ctx.JSON(http.StatusOK, res.data)
}

func (a *api) onRTSPSessionsList(ctx *gin.Context) {
	res := a.rtspServer.onAPISessionsList(rtspServerAPISessionsListReq{})
	if res.err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	ctx.JSON(http.StatusOK, res.data)
}

func (a *api) onRTSPSessionsKick(ctx *gin.Context) {
	id := ctx.Param("id")

	res := a.rtspServer.onAPISessionsKick(rtspServerAPISessionsKickReq{id: id})
	if res.err != nil {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *api) onRTSPSSessionsList(ctx *gin.Context) {
	res := a.rtspsServer.onAPISessionsList(rtspServerAPISessionsListReq{})
	if res.err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	ctx.JSON(http.StatusOK, res.data)
}

func (a *api) onRTSPSSessionsKick(ctx *gin.Context) {
	id := ctx.Param("id")

	res := a.rtspsServer.onAPISessionsKick(rtspServerAPISessionsKickReq{id: id})
	if res.err != nil {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *api) onRTMPConnsList(ctx *gin.Context) {
	res := a.rtmpServer.onAPIConnsList(rtmpServerAPIConnsListReq{})
	if res.err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	ctx.JSON(http.StatusOK, res.data)
}

func (a *api) onRTMPConnsKick(ctx *gin.Context) {
	id := ctx.Param("id")

	res := a.rtmpServer.onAPIConnsKick(rtmpServerAPIConnsKickReq{id: id})
	if res.err != nil {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *api) onHLSMuxersList(ctx *gin.Context) {
	res := a.hlsServer.onAPIHLSMuxersList(hlsServerAPIMuxersListReq{})
	if res.err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	ctx.JSON(http.StatusOK, res.data)
}

// onConfReload is called by core.
func (a *api) onConfReload(conf *conf.Conf) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.conf = conf
}
