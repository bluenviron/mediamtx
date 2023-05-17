package core

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
)

func interfaceIsEmpty(i interface{}) bool {
	return reflect.ValueOf(i).Kind() != reflect.Ptr || reflect.ValueOf(i).IsNil()
}

func fillStruct(dest interface{}, source interface{}) {
	rvsource := reflect.ValueOf(source).Elem()
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

func generateStructWithOptionalFields(model interface{}) interface{} {
	var fields []reflect.StructField

	rt := reflect.TypeOf(model)
	nf := rt.NumField()
	for i := 0; i < nf; i++ {
		f := rt.Field(i)
		j := f.Tag.Get("json")

		if j != "-" && j != "paths" {
			fields = append(fields, reflect.StructField{
				Name: f.Name,
				Type: reflect.PtrTo(f.Type),
				Tag:  f.Tag,
			})
		}
	}

	return reflect.New(reflect.StructOf(fields)).Interface()
}

func loadConfData(ctx *gin.Context) (interface{}, error) {
	in := generateStructWithOptionalFields(conf.Conf{})
	err := json.NewDecoder(ctx.Request.Body).Decode(in)
	if err != nil {
		return nil, err
	}

	return in, err
}

func loadConfPathData(ctx *gin.Context) (interface{}, error) {
	in := generateStructWithOptionalFields(conf.PathConf{})
	err := json.NewDecoder(ctx.Request.Body).Decode(in)
	if err != nil {
		return nil, err
	}

	return in, err
}

func paginate2(itemsPtr interface{}, itemsPerPage int, page int) int {
	ritems := reflect.ValueOf(itemsPtr).Elem()

	itemsLen := ritems.Len()
	if itemsLen == 0 {
		return 0
	}

	pageCount := (itemsLen / itemsPerPage) + 1

	min := page * itemsPerPage
	if min >= itemsLen {
		min = itemsLen - 1
	}

	max := (page + 1) * itemsPerPage
	if max >= itemsLen {
		max = itemsLen
	}

	ritems.Set(ritems.Slice(min, max))

	return pageCount
}

func paginate(itemsPtr interface{}, itemsPerPageStr string, pageStr string) (int, error) {
	itemsPerPage := 100

	if itemsPerPageStr != "" {
		tmp, err := strconv.ParseUint(itemsPerPageStr, 10, 31)
		if err != nil {
			return 0, err
		}
		itemsPerPage = int(tmp)
	}

	page := 0

	if pageStr != "" {
		tmp, err := strconv.ParseUint(pageStr, 10, 31)
		if err != nil {
			return 0, err
		}
		page = int(tmp)
	}

	return paginate2(itemsPtr, itemsPerPage, page), nil
}

type apiPathManager interface {
	apiPathsList() pathAPIPathsListRes
}

type apiHLSManager interface {
	apiMuxersList() hlsManagerAPIMuxersListRes
}

type apiRTSPServer interface {
	apiConnsList() rtspServerAPIConnsListRes
	apiSessionsList() rtspServerAPISessionsListRes
	apiSessionsKick(uuid.UUID) rtspServerAPISessionsKickRes
}

type apiRTMPServer interface {
	apiConnsList() rtmpServerAPIConnsListRes
	apiConnsKick(uuid.UUID) rtmpServerAPIConnsKickRes
}

type apiParent interface {
	logger.Writer
	apiConfigSet(conf *conf.Conf)
}

type apiWebRTCManager interface {
	apiSessionsList() webRTCManagerAPISessionsListRes
	apiSessionsKick(uuid.UUID) webRTCManagerAPISessionsKickRes
}

type api struct {
	conf          *conf.Conf
	pathManager   apiPathManager
	rtspServer    apiRTSPServer
	rtspsServer   apiRTSPServer
	rtmpServer    apiRTMPServer
	rtmpsServer   apiRTMPServer
	hlsManager    apiHLSManager
	webRTCManager apiWebRTCManager
	parent        apiParent

	httpServer *httpServer
	mutex      sync.Mutex
}

func newAPI(
	address string,
	readTimeout conf.StringDuration,
	conf *conf.Conf,
	pathManager apiPathManager,
	rtspServer apiRTSPServer,
	rtspsServer apiRTSPServer,
	rtmpServer apiRTMPServer,
	rtmpsServer apiRTMPServer,
	hlsManager apiHLSManager,
	webRTCManager apiWebRTCManager,
	parent apiParent,
) (*api, error) {
	a := &api{
		conf:          conf,
		pathManager:   pathManager,
		rtspServer:    rtspServer,
		rtspsServer:   rtspsServer,
		rtmpServer:    rtmpServer,
		rtmpsServer:   rtmpsServer,
		hlsManager:    hlsManager,
		webRTCManager: webRTCManager,
		parent:        parent,
	}

	router := gin.New()
	router.SetTrustedProxies(nil)

	mwLog := httpLoggerMiddleware(a)
	router.NoRoute(mwLog, httpServerHeaderMiddleware)
	group := router.Group("/", mwLog, httpServerHeaderMiddleware)

	group.GET("/v2/config/get", a.onConfigGet)
	group.POST("/v2/config/set", a.onConfigSet)
	group.POST("/v2/config/paths/add/*name", a.onConfigPathsAdd)
	group.POST("/v2/config/paths/edit/*name", a.onConfigPathsEdit)
	group.POST("/v2/config/paths/remove/*name", a.onConfigPathsDelete)

	if !interfaceIsEmpty(a.hlsManager) {
		group.GET("/v2/hlsmuxers/list", a.onHLSMuxersList)
	}

	group.GET("/v2/paths/list", a.onPathsList)

	if !interfaceIsEmpty(a.rtspServer) {
		group.GET("/v2/rtspconns/list", a.onRTSPConnsList)
		group.GET("/v2/rtspsessions/list", a.onRTSPSessionsList)
		group.POST("/v2/rtspsessions/kick/:id", a.onRTSPSessionsKick)
	}

	if !interfaceIsEmpty(a.rtspsServer) {
		group.GET("/v2/rtspsconns/list", a.onRTSPSConnsList)
		group.GET("/v2/rtspssessions/list", a.onRTSPSSessionsList)
		group.POST("/v2/rtspssessions/kick/:id", a.onRTSPSSessionsKick)
	}

	if !interfaceIsEmpty(a.rtmpServer) {
		group.GET("/v2/rtmpconns/list", a.onRTMPConnsList)
		group.POST("/v2/rtmpconns/kick/:id", a.onRTMPConnsKick)
	}

	if !interfaceIsEmpty(a.rtmpsServer) {
		group.GET("/v2/rtmpsconns/list", a.onRTMPSConnsList)
		group.POST("/v2/rtmpsconns/kick/:id", a.onRTMPSConnsKick)
	}

	if !interfaceIsEmpty(a.webRTCManager) {
		group.GET("/v2/webrtcsessions/list", a.onWebRTCSessionsList)
		group.POST("/v2/webrtcsessions/kick/:id", a.onWebRTCSessionsKick)
	}

	var err error
	a.httpServer, err = newHTTPServer(
		address,
		readTimeout,
		"",
		"",
		router,
	)
	if err != nil {
		return nil, err
	}

	a.Log(logger.Info, "listener opened on "+address)

	return a, nil
}

func (a *api) close() {
	a.Log(logger.Info, "listener is closing")
	a.httpServer.close()
}

func (a *api) Log(level logger.Level, format string, args ...interface{}) {
	a.parent.Log(level, "[API] "+format, args...)
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

	newConf := a.conf.Clone()

	fillStruct(newConf, in)

	err = newConf.Check()
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	a.conf = newConf

	// since reloading the configuration can cause the shutdown of the API,
	// call it in a goroutine
	go a.parent.apiConfigSet(newConf)

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

	newConf := a.conf.Clone()

	if _, ok := newConf.Paths[name]; ok {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	newConfPath := &conf.PathConf{}
	fillStruct(newConfPath, in)

	newConf.Paths[name] = newConfPath

	err = newConf.Check()
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	a.conf = newConf

	// since reloading the configuration can cause the shutdown of the API,
	// call it in a goroutine
	go a.parent.apiConfigSet(newConf)

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

	newConf := a.conf.Clone()

	newConfPath, ok := newConf.Paths[name]
	if !ok {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	fillStruct(newConfPath, in)

	err = newConf.Check()
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	a.conf = newConf

	// since reloading the configuration can cause the shutdown of the API,
	// call it in a goroutine
	go a.parent.apiConfigSet(newConf)

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

	newConf := a.conf.Clone()

	if _, ok := newConf.Paths[name]; !ok {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	delete(newConf.Paths, name)

	err := newConf.Check()
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	a.conf = newConf

	// since reloading the configuration can cause the shutdown of the API,
	// call it in a goroutine
	go a.parent.apiConfigSet(newConf)

	ctx.Status(http.StatusOK)
}

func (a *api) onPathsList(ctx *gin.Context) {
	res := a.pathManager.apiPathsList()
	if res.err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	pageCount, err := paginate(&res.data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	res.data.PageCount = pageCount

	ctx.JSON(http.StatusOK, res.data)
}

func (a *api) onRTSPConnsList(ctx *gin.Context) {
	res := a.rtspServer.apiConnsList()
	if res.err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	pageCount, err := paginate(&res.data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	res.data.PageCount = pageCount

	ctx.JSON(http.StatusOK, res.data)
}

func (a *api) onRTSPSessionsList(ctx *gin.Context) {
	res := a.rtspServer.apiSessionsList()
	if res.err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	pageCount, err := paginate(&res.data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	res.data.PageCount = pageCount

	ctx.JSON(http.StatusOK, res.data)
}

func (a *api) onRTSPSessionsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	res := a.rtspServer.apiSessionsKick(uuid)
	if res.err != nil {
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *api) onRTSPSConnsList(ctx *gin.Context) {
	res := a.rtspsServer.apiConnsList()
	if res.err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	pageCount, err := paginate(&res.data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	res.data.PageCount = pageCount

	ctx.JSON(http.StatusOK, res.data)
}

func (a *api) onRTSPSSessionsList(ctx *gin.Context) {
	res := a.rtspsServer.apiSessionsList()
	if res.err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	pageCount, err := paginate(&res.data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	res.data.PageCount = pageCount

	ctx.JSON(http.StatusOK, res.data)
}

func (a *api) onRTSPSSessionsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	res := a.rtspsServer.apiSessionsKick(uuid)
	if res.err != nil {
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *api) onRTMPConnsList(ctx *gin.Context) {
	res := a.rtmpServer.apiConnsList()
	if res.err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	pageCount, err := paginate(&res.data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	res.data.PageCount = pageCount

	ctx.JSON(http.StatusOK, res.data)
}

func (a *api) onRTMPConnsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	res := a.rtmpServer.apiConnsKick(uuid)
	if res.err != nil {
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *api) onRTMPSConnsList(ctx *gin.Context) {
	res := a.rtmpsServer.apiConnsList()
	if res.err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	pageCount, err := paginate(&res.data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	res.data.PageCount = pageCount

	ctx.JSON(http.StatusOK, res.data)
}

func (a *api) onRTMPSConnsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	res := a.rtmpsServer.apiConnsKick(uuid)
	if res.err != nil {
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *api) onHLSMuxersList(ctx *gin.Context) {
	res := a.hlsManager.apiMuxersList()
	if res.err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	pageCount, err := paginate(&res.data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	res.data.PageCount = pageCount

	ctx.JSON(http.StatusOK, res.data)
}

func (a *api) onWebRTCSessionsList(ctx *gin.Context) {
	res := a.webRTCManager.apiSessionsList()
	if res.err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	pageCount, err := paginate(&res.data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	res.data.PageCount = pageCount

	ctx.JSON(http.StatusOK, res.data)
}

func (a *api) onWebRTCSessionsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	res := a.webRTCManager.apiSessionsKick(uuid)
	if res.err != nil {
		return
	}

	ctx.Status(http.StatusOK)
}

// confReload is called by core.
func (a *api) confReload(conf *conf.Conf) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.conf = conf
}
