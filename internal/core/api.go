package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"reflect"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

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
	enc, _ := json.Marshal(dest)
	_ = json.Unmarshal(enc, source)
}

func loadConfData(ctx *gin.Context) (interface{}, error) {
	var in struct {
		// general
		LogLevel            *string        `json:"logLevel"`
		LogDestinations     *[]string      `json:"logDestinations"`
		LogFile             *string        `json:"logFile"`
		ReadTimeout         *time.Duration `json:"readTimeout"`
		WriteTimeout        *time.Duration `json:"writeTimeout"`
		ReadBufferCount     *int           `json:"readBufferCount"`
		API                 *bool          `json:"api"`
		APIAddress          *string        `json:"apiAddress"`
		Metrics             *bool          `json:"metrics"`
		MetricsAddress      *string        `json:"metricsAddress"`
		PPROF               *bool          `json:"pprof"`
		PPROFAddress        *string        `json:"pprofAddress"`
		RunOnConnect        *string        `json:"runOnConnect"`
		RunOnConnectRestart *bool          `json:"runOnConnectRestart"`

		// rtsp
		RTSPDisable       *bool     `json:"rtspDisable"`
		Protocols         *[]string `json:"protocols"`
		Encryption        *string   `json:"encryption"`
		RTSPAddress       *string   `json:"rtspAddress"`
		RTSPSAddress      *string   `json:"rtspsAddress"`
		RTPAddress        *string   `json:"rtpAddress"`
		RTCPAddress       *string   `json:"rtcpAddress"`
		MulticastIPRange  *string   `json:"multicastIPRange"`
		MulticastRTPPort  *int      `json:"multicastRTPPort"`
		MulticastRTCPPort *int      `json:"multicastRTCPPort"`
		ServerKey         *string   `json:"serverKey"`
		ServerCert        *string   `json:"serverCert"`
		AuthMethods       *[]string `json:"authMethods"`
		ReadBufferSize    *int      `json:"readBufferSize"`

		// rtmp
		RTMPDisable *bool   `json:"rtmpDisable"`
		RTMPAddress *string `json:"rtmpAddress"`

		// hls
		HLSDisable         *bool          `json:"hlsDisable"`
		HLSAddress         *string        `json:"hlsAddress"`
		HLSAlwaysRemux     *bool          `json:"hlsAlwaysRemux"`
		HLSSegmentCount    *int           `json:"hlsSegmentCount"`
		HLSSegmentDuration *time.Duration `json:"hlsSegmentDuration"`
		HLSAllowOrigin     *string        `json:"hlsAllowOrigin"`
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
		Source                     *string        `json:"source"`
		SourceProtocol             *string        `json:"sourceProtocol"`
		SourceAnyPortEnable        *bool          `json:"sourceAnyPortEnable"`
		SourceFingerprint          *string        `json:"sourceFingerprint"`
		SourceOnDemand             *bool          `json:"sourceOnDemand"`
		SourceOnDemandStartTimeout *time.Duration `json:"sourceOnDemandStartTimeout"`
		SourceOnDemandCloseAfter   *time.Duration `json:"sourceOnDemandCloseAfter"`
		SourceRedirect             *string        `json:"sourceRedirect"`
		DisablePublisherOverride   *bool          `json:"disablePublisherOverride"`
		Fallback                   *string        `json:"fallback"`

		// authentication
		PublishUser *string   `json:"publishUser"`
		PublishPass *string   `json:"publishPass"`
		PublishIPs  *[]string `json:"publishIPs"`
		ReadUser    *string   `json:"readUser"`
		ReadPass    *string   `json:"readPass"`
		ReadIPs     *[]string `json:"readIPs"`

		// custom commands
		RunOnInit               *string        `json:"runOnInit"`
		RunOnInitRestart        *bool          `json:"runOnInitRestart"`
		RunOnDemand             *string        `json:"runOnDemand"`
		RunOnDemandRestart      *bool          `json:"runOnDemandRestart"`
		RunOnDemandStartTimeout *time.Duration `json:"runOnDemandStartTimeout"`
		RunOnDemandCloseAfter   *time.Duration `json:"runOnDemandCloseAfter"`
		RunOnPublish            *string        `json:"runOnPublish"`
		RunOnPublishRestart     *bool          `json:"runOnPublishRestart"`
		RunOnRead               *string        `json:"runOnRead"`
		RunOnReadRestart        *bool          `json:"runOnReadRestart"`
	}
	err := json.NewDecoder(ctx.Request.Body).Decode(&in)
	if err != nil {
		return nil, err
	}

	return in, err
}

type apiPathsItem struct {
	ConfName    string         `json:"confName"`
	Conf        *conf.PathConf `json:"conf"`
	Source      interface{}    `json:"source"`
	SourceReady bool           `json:"sourceReady"`
	Readers     []interface{}  `json:"readers"`
}

type apiPathsListData struct {
	Items map[string]apiPathsItem `json:"items"`
}

type apiPathsListRes1 struct {
	Paths map[string]*path
	Err   error
}

type apiPathsListReq1 struct {
	Res chan apiPathsListRes1
}

type apiPathsListRes2 struct {
	Err error
}

type apiPathsListReq2 struct {
	Data *apiPathsListData
	Res  chan apiPathsListRes2
}

type apiRTSPSessionsListItem struct {
	RemoteAddr string `json:"remoteAddr"`
	State      string `json:"state"`
}

type apiRTSPSessionsListData struct {
	Items map[string]apiRTSPSessionsListItem `json:"items"`
}

type apiRTSPSessionsListRes struct {
	Err error
}

type apiRTSPSessionsListReq struct {
	Data *apiRTSPSessionsListData
}

type apiRTSPSessionsKickRes struct {
	Err error
}

type apiRTSPSessionsKickReq struct {
	ID string
}

type apiRTMPConnsListItem struct {
	RemoteAddr string `json:"remoteAddr"`
	State      string `json:"state"`
}

type apiRTMPConnsListData struct {
	Items map[string]apiRTMPConnsListItem `json:"items"`
}

type apiRTMPConnsListRes struct {
	Err error
}

type apiRTMPConnsListReq struct {
	Data *apiRTMPConnsListData
	Res  chan apiRTMPConnsListRes
}

type apiRTMPConnsKickRes struct {
	Err error
}

type apiRTMPConnsKickReq struct {
	ID  string
	Res chan apiRTMPConnsKickRes
}

type apiParent interface {
	Log(logger.Level, string, ...interface{})
	OnAPIConfigSet(conf *conf.Conf)
}

type api struct {
	conf        *conf.Conf
	pathManager *pathManager
	rtspServer  *rtspServer
	rtspsServer *rtspServer
	rtmpServer  *rtmpServer
	parent      apiParent

	mutex sync.Mutex
	s     *http.Server
}

func newAPI(
	address string,
	conf *conf.Conf,
	pathManager *pathManager,
	rtspServer *rtspServer,
	rtspsServer *rtspServer,
	rtmpServer *rtmpServer,
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
		parent:      parent,
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	group := router.Group("/", a.mwLog)
	group.GET("/v1/config/get", a.onConfigGet)
	group.POST("/v1/config/set", a.onConfigSet)
	group.POST("/v1/config/paths/add/:name", a.onConfigPathsAdd)
	group.POST("/v1/config/paths/edit/:name", a.onConfigPathsEdit)
	group.POST("/v1/config/paths/remove/:name", a.onConfigPathsDelete)
	group.GET("/v1/paths/list", a.onPathsList)
	group.GET("/v1/rtspsessions/list", a.onRTSPSessionsList)
	group.POST("/v1/rtspsessions/kick/:id", a.onRTSPSessionsKick)
	group.GET("/v1/rtmpconns/list", a.onRTMPConnsList)
	group.POST("/v1/rtmpconns/kick/:id", a.onRTMPConnsKick)

	a.s = &http.Server{
		Handler: router,
	}

	go a.s.Serve(ln)

	a.log(logger.Info, "listener opened on "+address)

	return a, nil
}

func (a *api) close() {
	a.s.Shutdown(context.Background())
	a.log(logger.Info, "closed")
}

// Log is the main logging function.
func (a *api) log(level logger.Level, format string, args ...interface{}) {
	a.parent.Log(level, "[API] "+format, args...)
}

type logWriter struct {
	gin.ResponseWriter
	buf bytes.Buffer
}

func (w *logWriter) Write(b []byte) (int, error) {
	w.buf.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *logWriter) WriteString(s string) (int, error) {
	w.buf.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

func (a *api) mwLog(ctx *gin.Context) {
	byts, _ := httputil.DumpRequest(ctx.Request, true)
	a.log(logger.Debug, "[c->s] %s", string(byts))

	blw := &logWriter{ResponseWriter: ctx.Writer}
	ctx.Writer = blw

	ctx.Next()

	ctx.Writer.Header().Set("Server", "rtsp-simple-server")

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "HTTP/1.1 %d %s\n", ctx.Writer.Status(), http.StatusText(ctx.Writer.Status()))
	ctx.Writer.Header().Write(&buf)
	buf.Write([]byte("\n"))
	buf.Write(blw.buf.Bytes())

	a.log(logger.Debug, "[s->c] %s", buf.String())
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
	var newConf conf.Conf
	cloneStruct(a.conf, &newConf)
	a.mutex.Unlock()

	fillStruct(&newConf, in)

	err = newConf.CheckAndFillMissing()
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	a.mutex.Lock()
	a.conf = &newConf
	a.mutex.Unlock()

	// since reloading the configuration can cause the shutdown of the API,
	// call it in a goroutine
	go a.parent.OnAPIConfigSet(&newConf)

	ctx.Status(http.StatusOK)
}

func (a *api) onConfigPathsAdd(ctx *gin.Context) {
	in, err := loadConfPathData(ctx)
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	name := ctx.Param("name")

	a.mutex.Lock()
	var newConf conf.Conf
	cloneStruct(a.conf, &newConf)
	a.mutex.Unlock()

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

	a.mutex.Lock()
	a.conf = &newConf
	a.mutex.Unlock()

	// since reloading the configuration can cause the shutdown of the API,
	// call it in a goroutine
	go a.parent.OnAPIConfigSet(&newConf)

	ctx.Status(http.StatusOK)
}

func (a *api) onConfigPathsEdit(ctx *gin.Context) {
	in, err := loadConfPathData(ctx)
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	name := ctx.Param("name")

	a.mutex.Lock()
	var newConf conf.Conf
	cloneStruct(a.conf, &newConf)
	a.mutex.Unlock()

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

	a.mutex.Lock()
	a.conf = &newConf
	a.mutex.Unlock()

	// since reloading the configuration can cause the shutdown of the API,
	// call it in a goroutine
	go a.parent.OnAPIConfigSet(&newConf)

	ctx.Status(http.StatusOK)
}

func (a *api) onConfigPathsDelete(ctx *gin.Context) {
	name := ctx.Param("name")

	a.mutex.Lock()
	var newConf conf.Conf
	cloneStruct(a.conf, &newConf)
	a.mutex.Unlock()

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

	a.mutex.Lock()
	a.conf = &newConf
	a.mutex.Unlock()

	// since reloading the configuration can cause the shutdown of the API,
	// call it in a goroutine
	go a.parent.OnAPIConfigSet(&newConf)

	ctx.Status(http.StatusOK)
}

func (a *api) onPathsList(ctx *gin.Context) {
	data := apiPathsListData{
		Items: make(map[string]apiPathsItem),
	}

	res := a.pathManager.OnAPIPathsList(apiPathsListReq1{})
	if res.Err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	for _, pa := range res.Paths {
		pa.OnAPIPathsList(apiPathsListReq2{Data: &data})
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTSPSessionsList(ctx *gin.Context) {
	if a.rtspServer == nil && a.rtspsServer == nil {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}

	data := apiRTSPSessionsListData{
		Items: make(map[string]apiRTSPSessionsListItem),
	}

	if a.rtspServer != nil {
		res := a.rtspServer.OnAPIRTSPSessionsList(apiRTSPSessionsListReq{Data: &data})
		if res.Err != nil {
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}
	}

	if a.rtspsServer != nil {
		res := a.rtspsServer.OnAPIRTSPSessionsList(apiRTSPSessionsListReq{Data: &data})
		if res.Err != nil {
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTSPSessionsKick(ctx *gin.Context) {
	if a.rtspServer == nil && a.rtspsServer == nil {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}

	id := ctx.Param("id")

	if a.rtspServer != nil {
		res := a.rtspServer.OnAPIRTSPSessionsKick(apiRTSPSessionsKickReq{ID: id})
		if res.Err == nil {
			ctx.Status(http.StatusOK)
			return
		}
	}

	if a.rtspsServer != nil {
		res := a.rtspsServer.OnAPIRTSPSessionsKick(apiRTSPSessionsKickReq{ID: id})
		if res.Err != nil {
			ctx.Status(http.StatusOK)
			return
		}
	}

	ctx.AbortWithStatus(http.StatusNotFound)
}

func (a *api) onRTMPConnsList(ctx *gin.Context) {
	if a.rtmpServer == nil {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}

	data := apiRTMPConnsListData{
		Items: make(map[string]apiRTMPConnsListItem),
	}

	res := a.rtmpServer.OnAPIRTMPConnsList(apiRTMPConnsListReq{Data: &data})
	if res.Err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	ctx.JSON(http.StatusOK, data)
}

// OnConfReload is called by core.
func (a *api) OnConfReload(conf *conf.Conf) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.conf = conf
}

func (a *api) onRTMPConnsKick(ctx *gin.Context) {
	if a.rtmpServer == nil {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}

	id := ctx.Param("id")

	res := a.rtmpServer.OnAPIRTMPConnsKick(apiRTMPConnsKickReq{ID: id})
	if res.Err != nil {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}

	ctx.Status(http.StatusOK)
}
