package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpserv"
)

func interfaceIsEmpty(i interface{}) bool {
	return reflect.ValueOf(i).Kind() != reflect.Ptr || reflect.ValueOf(i).IsNil()
}

func paginate2(itemsPtr interface{}, itemsPerPage int, page int) int {
	ritems := reflect.ValueOf(itemsPtr).Elem()

	itemsLen := ritems.Len()
	if itemsLen == 0 {
		return 0
	}

	pageCount := (itemsLen / itemsPerPage)
	if (itemsLen % itemsPerPage) != 0 {
		pageCount++
	}

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

func sortedKeys(paths map[string]*conf.OptionalPath) []string {
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

type apiPathManager interface {
	apiPathsList() (*apiPathList, error)
	apiPathsGet(string) (*apiPath, error)
}

type apiHLSManager interface {
	apiMuxersList() (*apiHLSMuxerList, error)
	apiMuxersGet(string) (*apiHLSMuxer, error)
}

type apiRTSPServer interface {
	apiConnsList() (*apiRTSPConnsList, error)
	apiConnsGet(uuid.UUID) (*apiRTSPConn, error)
	apiSessionsList() (*apiRTSPSessionList, error)
	apiSessionsGet(uuid.UUID) (*apiRTSPSession, error)
	apiSessionsKick(uuid.UUID) error
}

type apiRTMPServer interface {
	apiConnsList() (*apiRTMPConnList, error)
	apiConnsGet(uuid.UUID) (*apiRTMPConn, error)
	apiConnsKick(uuid.UUID) error
}

type apiWebRTCManager interface {
	apiSessionsList() (*apiWebRTCSessionList, error)
	apiSessionsGet(uuid.UUID) (*apiWebRTCSession, error)
	apiSessionsKick(uuid.UUID) error
}

type apiSRTServer interface {
	apiConnsList() (*apiSRTConnList, error)
	apiConnsGet(uuid.UUID) (*apiSRTConn, error)
	apiConnsKick(uuid.UUID) error
}

type apiParent interface {
	logger.Writer
	apiConfigSet(conf *conf.Conf)
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
	srtServer     apiSRTServer
	parent        apiParent

	httpServer *httpserv.WrappedServer
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
	srtServer apiSRTServer,
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
		srtServer:     srtServer,
		parent:        parent,
	}

	router := gin.New()
	router.SetTrustedProxies(nil) //nolint:errcheck

	group := router.Group("/")

	group.GET("/v3/config/global/get", a.onConfigGlobalGet)
	group.PATCH("/v3/config/global/patch", a.onConfigGlobalPatch)

	group.GET("/v3/config/pathdefaults/get", a.onConfigPathDefaultsGet)
	group.PATCH("/v3/config/pathdefaults/patch", a.onConfigPathDefaultsPatch)

	group.GET("/v3/config/paths/list", a.onConfigPathsList)
	group.GET("/v3/config/paths/get/*name", a.onConfigPathsGet)
	group.POST("/v3/config/paths/add/*name", a.onConfigPathsAdd)
	group.PATCH("/v3/config/paths/patch/*name", a.onConfigPathsPatch)
	group.POST("/v3/config/paths/replace/*name", a.onConfigPathsReplace)
	group.DELETE("/v3/config/paths/delete/*name", a.onConfigPathsDelete)

	group.GET("/v3/paths/list", a.onPathsList)
	group.GET("/v3/paths/get/*name", a.onPathsGet)

	if !interfaceIsEmpty(a.hlsManager) {
		group.GET("/v3/hlsmuxers/list", a.onHLSMuxersList)
		group.GET("/v3/hlsmuxers/get/*name", a.onHLSMuxersGet)
	}

	if !interfaceIsEmpty(a.rtspServer) {
		group.GET("/v3/rtspconns/list", a.onRTSPConnsList)
		group.GET("/v3/rtspconns/get/:id", a.onRTSPConnsGet)
		group.GET("/v3/rtspsessions/list", a.onRTSPSessionsList)
		group.GET("/v3/rtspsessions/get/:id", a.onRTSPSessionsGet)
		group.POST("/v3/rtspsessions/kick/:id", a.onRTSPSessionsKick)
	}

	if !interfaceIsEmpty(a.rtspsServer) {
		group.GET("/v3/rtspsconns/list", a.onRTSPSConnsList)
		group.GET("/v3/rtspsconns/get/:id", a.onRTSPSConnsGet)
		group.GET("/v3/rtspssessions/list", a.onRTSPSSessionsList)
		group.GET("/v3/rtspssessions/get/:id", a.onRTSPSSessionsGet)
		group.POST("/v3/rtspssessions/kick/:id", a.onRTSPSSessionsKick)
	}

	if !interfaceIsEmpty(a.rtmpServer) {
		group.GET("/v3/rtmpconns/list", a.onRTMPConnsList)
		group.GET("/v3/rtmpconns/get/:id", a.onRTMPConnsGet)
		group.POST("/v3/rtmpconns/kick/:id", a.onRTMPConnsKick)
	}

	if !interfaceIsEmpty(a.rtmpsServer) {
		group.GET("/v3/rtmpsconns/list", a.onRTMPSConnsList)
		group.GET("/v3/rtmpsconns/get/:id", a.onRTMPSConnsGet)
		group.POST("/v3/rtmpsconns/kick/:id", a.onRTMPSConnsKick)
	}

	if !interfaceIsEmpty(a.webRTCManager) {
		group.GET("/v3/webrtcsessions/list", a.onWebRTCSessionsList)
		group.GET("/v3/webrtcsessions/get/:id", a.onWebRTCSessionsGet)
		group.POST("/v3/webrtcsessions/kick/:id", a.onWebRTCSessionsKick)
	}

	if !interfaceIsEmpty(a.srtServer) {
		group.GET("/v3/srtconns/list", a.onSRTConnsList)
		group.GET("/v3/srtconns/get/:id", a.onSRTConnsGet)
		group.POST("/v3/srtconns/kick/:id", a.onSRTConnsKick)
	}

	network, address := restrictNetwork("tcp", address)

	var err error
	a.httpServer, err = httpserv.NewWrappedServer(
		network,
		address,
		time.Duration(readTimeout),
		"",
		"",
		router,
		a,
	)
	if err != nil {
		return nil, err
	}

	a.Log(logger.Info, "listener opened on "+address)

	return a, nil
}

func (a *api) close() {
	a.Log(logger.Info, "listener is closing")
	a.httpServer.Close()
}

func (a *api) Log(level logger.Level, format string, args ...interface{}) {
	a.parent.Log(level, "[API] "+format, args...)
}

// error coming from something the user inserted into the request.
func (a *api) writeUserError(ctx *gin.Context, err error) {
	// show error in logs
	a.Log(logger.Error, err.Error())

	// send error in response
	ctx.JSON(http.StatusBadRequest, &apiError{
		Error: err.Error(),
	})
}

// error coming from the server.
func (a *api) writeServerError(ctx *gin.Context, err error) {
	// show error in logs
	a.Log(logger.Error, err.Error())

	// send error in response
	ctx.JSON(http.StatusInternalServerError, &apiError{
		Error: err.Error(),
	})
}

func (a *api) onConfigGlobalGet(ctx *gin.Context) {
	a.mutex.Lock()
	c := a.conf
	a.mutex.Unlock()

	ctx.JSON(http.StatusOK, c.Global())
}

func (a *api) onConfigGlobalPatch(ctx *gin.Context) {
	var c conf.OptionalGlobal
	err := json.NewDecoder(ctx.Request.Body).Decode(&c)
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	newConf := a.conf.Clone()

	newConf.PatchGlobal(&c)

	err = newConf.Check()
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	a.conf = newConf

	// since reloading the configuration can cause the shutdown of the API,
	// call it in a goroutine
	go a.parent.apiConfigSet(newConf)

	ctx.Status(http.StatusOK)
}

func (a *api) onConfigPathDefaultsGet(ctx *gin.Context) {
	a.mutex.Lock()
	c := a.conf
	a.mutex.Unlock()

	ctx.JSON(http.StatusOK, c.PathDefaults)
}

func (a *api) onConfigPathDefaultsPatch(ctx *gin.Context) {
	var p conf.OptionalPath
	err := json.NewDecoder(ctx.Request.Body).Decode(&p)
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	newConf := a.conf.Clone()

	newConf.PatchPathDefaults(&p)

	err = newConf.Check()
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	a.conf = newConf
	a.parent.apiConfigSet(newConf)

	ctx.Status(http.StatusOK)
}

func (a *api) onConfigPathsList(ctx *gin.Context) {
	a.mutex.Lock()
	c := a.conf
	a.mutex.Unlock()

	data := &apiPathConfList{
		Items: make([]*conf.OptionalPath, len(c.OptionalPaths)),
	}

	for i, key := range sortedKeys(c.OptionalPaths) {
		data.Items[i] = c.OptionalPaths[key]
	}

	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onConfigPathsGet(ctx *gin.Context) {
	name, ok := paramName(ctx)
	if !ok {
		a.writeUserError(ctx, fmt.Errorf("invalid name"))
		return
	}

	a.mutex.Lock()
	c := a.conf
	a.mutex.Unlock()

	p, ok := c.OptionalPaths[name]
	if !ok {
		a.writeServerError(ctx, fmt.Errorf("path configuration not found"))
		return
	}

	ctx.JSON(http.StatusOK, p)
}

func (a *api) onConfigPathsAdd(ctx *gin.Context) { //nolint:dupl
	name, ok := paramName(ctx)
	if !ok {
		a.writeUserError(ctx, fmt.Errorf("invalid name"))
		return
	}

	var p conf.OptionalPath
	err := json.NewDecoder(ctx.Request.Body).Decode(&p)
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	newConf := a.conf.Clone()

	err = newConf.AddPath(name, &p)
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	err = newConf.Check()
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	a.conf = newConf
	a.parent.apiConfigSet(newConf)

	ctx.Status(http.StatusOK)
}

func (a *api) onConfigPathsPatch(ctx *gin.Context) { //nolint:dupl
	name, ok := paramName(ctx)
	if !ok {
		a.writeUserError(ctx, fmt.Errorf("invalid name"))
		return
	}

	var p conf.OptionalPath
	err := json.NewDecoder(ctx.Request.Body).Decode(&p)
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	newConf := a.conf.Clone()

	err = newConf.PatchPath(name, &p)
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	err = newConf.Check()
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	a.conf = newConf
	a.parent.apiConfigSet(newConf)

	ctx.Status(http.StatusOK)
}

func (a *api) onConfigPathsReplace(ctx *gin.Context) { //nolint:dupl
	name, ok := paramName(ctx)
	if !ok {
		a.writeUserError(ctx, fmt.Errorf("invalid name"))
		return
	}

	var p conf.OptionalPath
	err := json.NewDecoder(ctx.Request.Body).Decode(&p)
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	newConf := a.conf.Clone()

	err = newConf.ReplacePath(name, &p)
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	err = newConf.Check()
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	a.conf = newConf
	a.parent.apiConfigSet(newConf)

	ctx.Status(http.StatusOK)
}

func (a *api) onConfigPathsDelete(ctx *gin.Context) {
	name, ok := paramName(ctx)
	if !ok {
		a.writeUserError(ctx, fmt.Errorf("invalid name"))
		return
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	newConf := a.conf.Clone()

	err := newConf.RemovePath(name)
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	err = newConf.Check()
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	a.conf = newConf
	a.parent.apiConfigSet(newConf)

	ctx.Status(http.StatusOK)
}

func (a *api) onPathsList(ctx *gin.Context) {
	data, err := a.pathManager.apiPathsList()
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onPathsGet(ctx *gin.Context) {
	name, ok := paramName(ctx)
	if !ok {
		a.writeUserError(ctx, fmt.Errorf("invalid name"))
		return
	}

	data, err := a.pathManager.apiPathsGet(name)
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTSPConnsList(ctx *gin.Context) {
	data, err := a.rtspServer.apiConnsList()
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTSPConnsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	data, err := a.rtspServer.apiConnsGet(uuid)
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTSPSessionsList(ctx *gin.Context) {
	data, err := a.rtspServer.apiSessionsList()
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTSPSessionsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	data, err := a.rtspServer.apiSessionsGet(uuid)
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTSPSessionsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	err = a.rtspServer.apiSessionsKick(uuid)
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *api) onRTSPSConnsList(ctx *gin.Context) {
	data, err := a.rtspsServer.apiConnsList()
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTSPSConnsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	data, err := a.rtspsServer.apiConnsGet(uuid)
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTSPSSessionsList(ctx *gin.Context) {
	data, err := a.rtspsServer.apiSessionsList()
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTSPSSessionsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	data, err := a.rtspsServer.apiSessionsGet(uuid)
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTSPSSessionsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	err = a.rtspsServer.apiSessionsKick(uuid)
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *api) onRTMPConnsList(ctx *gin.Context) {
	data, err := a.rtmpServer.apiConnsList()
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTMPConnsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	data, err := a.rtmpServer.apiConnsGet(uuid)
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTMPConnsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	err = a.rtmpServer.apiConnsKick(uuid)
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *api) onRTMPSConnsList(ctx *gin.Context) {
	data, err := a.rtmpsServer.apiConnsList()
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTMPSConnsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	data, err := a.rtmpsServer.apiConnsGet(uuid)
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTMPSConnsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	err = a.rtmpsServer.apiConnsKick(uuid)
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *api) onHLSMuxersList(ctx *gin.Context) {
	data, err := a.hlsManager.apiMuxersList()
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onHLSMuxersGet(ctx *gin.Context) {
	name, ok := paramName(ctx)
	if !ok {
		a.writeUserError(ctx, fmt.Errorf("invalid name"))
		return
	}

	data, err := a.hlsManager.apiMuxersGet(name)
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onWebRTCSessionsList(ctx *gin.Context) {
	data, err := a.webRTCManager.apiSessionsList()
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onWebRTCSessionsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	data, err := a.webRTCManager.apiSessionsGet(uuid)
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onWebRTCSessionsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	err = a.webRTCManager.apiSessionsKick(uuid)
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *api) onSRTConnsList(ctx *gin.Context) {
	data, err := a.srtServer.apiConnsList()
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onSRTConnsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	data, err := a.srtServer.apiConnsGet(uuid)
	if err != nil {
		a.writeServerError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onSRTConnsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeUserError(ctx, err)
		return
	}

	err = a.srtServer.apiConnsKick(uuid)
	if err != nil {
		a.writeServerError(ctx, err)
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
