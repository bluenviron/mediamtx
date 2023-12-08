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
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpserv"
	"github.com/bluenviron/mediamtx/internal/restrictnetwork"
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
	if min > itemsLen {
		min = itemsLen
	}

	max := (page + 1) * itemsPerPage
	if max > itemsLen {
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

type apiPathManager interface {
	apiPathsList() (*defs.APIPathList, error)
	apiPathsGet(string) (*defs.APIPath, error)
}

type apiHLSServer interface {
	APIMuxersList() (*defs.APIHLSMuxerList, error)
	APIMuxersGet(string) (*defs.APIHLSMuxer, error)
}

type apiRTSPServer interface {
	APIConnsList() (*defs.APIRTSPConnsList, error)
	APIConnsGet(uuid.UUID) (*defs.APIRTSPConn, error)
	APISessionsList() (*defs.APIRTSPSessionList, error)
	APISessionsGet(uuid.UUID) (*defs.APIRTSPSession, error)
	APISessionsKick(uuid.UUID) error
}

type apiRTMPServer interface {
	APIConnsList() (*defs.APIRTMPConnList, error)
	APIConnsGet(uuid.UUID) (*defs.APIRTMPConn, error)
	APIConnsKick(uuid.UUID) error
}

type apiSRTServer interface {
	APIConnsList() (*defs.APISRTConnList, error)
	APIConnsGet(uuid.UUID) (*defs.APISRTConn, error)
	APIConnsKick(uuid.UUID) error
}

type apiWebRTCServer interface {
	APISessionsList() (*defs.APIWebRTCSessionList, error)
	APISessionsGet(uuid.UUID) (*defs.APIWebRTCSession, error)
	APISessionsKick(uuid.UUID) error
}

type apiParent interface {
	logger.Writer
	apiConfigSet(conf *conf.Conf)
}

type api struct {
	conf         *conf.Conf
	pathManager  apiPathManager
	rtspServer   apiRTSPServer
	rtspsServer  apiRTSPServer
	rtmpServer   apiRTMPServer
	rtmpsServer  apiRTMPServer
	hlsManager   apiHLSServer
	webRTCServer apiWebRTCServer
	srtServer    apiSRTServer
	parent       apiParent

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
	hlsManager apiHLSServer,
	webRTCServer apiWebRTCServer,
	srtServer apiSRTServer,
	parent apiParent,
) (*api, error) {
	a := &api{
		conf:         conf,
		pathManager:  pathManager,
		rtspServer:   rtspServer,
		rtspsServer:  rtspsServer,
		rtmpServer:   rtmpServer,
		rtmpsServer:  rtmpsServer,
		hlsManager:   hlsManager,
		webRTCServer: webRTCServer,
		srtServer:    srtServer,
		parent:       parent,
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

	if !interfaceIsEmpty(a.webRTCServer) {
		group.GET("/v3/webrtcsessions/list", a.onWebRTCSessionsList)
		group.GET("/v3/webrtcsessions/get/:id", a.onWebRTCSessionsGet)
		group.POST("/v3/webrtcsessions/kick/:id", a.onWebRTCSessionsKick)
	}

	if !interfaceIsEmpty(a.srtServer) {
		group.GET("/v3/srtconns/list", a.onSRTConnsList)
		group.GET("/v3/srtconns/get/:id", a.onSRTConnsGet)
		group.POST("/v3/srtconns/kick/:id", a.onSRTConnsKick)
	}

	network, address := restrictnetwork.Restrict("tcp", address)

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

// Log implements logger.Writer.
func (a *api) Log(level logger.Level, format string, args ...interface{}) {
	a.parent.Log(level, "[API] "+format, args...)
}

// error coming from something the user inserted into the request.
func (a *api) writeError(ctx *gin.Context, status int, err error) {
	// show error in logs
	a.Log(logger.Error, err.Error())

	// send error in response
	ctx.JSON(status, &defs.APIError{
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
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	newConf := a.conf.Clone()

	newConf.PatchGlobal(&c)

	err = newConf.Check()
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
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
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	newConf := a.conf.Clone()

	newConf.PatchPathDefaults(&p)

	err = newConf.Check()
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
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

	data := &defs.APIPathConfList{
		Items: make([]*conf.Path, len(c.Paths)),
	}

	for i, key := range sortedKeys(c.Paths) {
		data.Items[i] = c.Paths[key]
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

func (a *api) onConfigPathsGet(ctx *gin.Context) {
	name, ok := paramName(ctx)
	if !ok {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid name"))
		return
	}

	a.mutex.Lock()
	c := a.conf
	a.mutex.Unlock()

	p, ok := c.Paths[name]
	if !ok {
		a.writeError(ctx, http.StatusInternalServerError, fmt.Errorf("path configuration not found"))
		return
	}

	ctx.JSON(http.StatusOK, p)
}

func (a *api) onConfigPathsAdd(ctx *gin.Context) { //nolint:dupl
	name, ok := paramName(ctx)
	if !ok {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid name"))
		return
	}

	var p conf.OptionalPath
	err := json.NewDecoder(ctx.Request.Body).Decode(&p)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	newConf := a.conf.Clone()

	err = newConf.AddPath(name, &p)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = newConf.Check()
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	a.conf = newConf
	a.parent.apiConfigSet(newConf)

	ctx.Status(http.StatusOK)
}

func (a *api) onConfigPathsPatch(ctx *gin.Context) { //nolint:dupl
	name, ok := paramName(ctx)
	if !ok {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid name"))
		return
	}

	var p conf.OptionalPath
	err := json.NewDecoder(ctx.Request.Body).Decode(&p)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	newConf := a.conf.Clone()

	err = newConf.PatchPath(name, &p)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = newConf.Check()
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	a.conf = newConf
	a.parent.apiConfigSet(newConf)

	ctx.Status(http.StatusOK)
}

func (a *api) onConfigPathsReplace(ctx *gin.Context) { //nolint:dupl
	name, ok := paramName(ctx)
	if !ok {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid name"))
		return
	}

	var p conf.OptionalPath
	err := json.NewDecoder(ctx.Request.Body).Decode(&p)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	newConf := a.conf.Clone()

	err = newConf.ReplacePath(name, &p)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = newConf.Check()
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	a.conf = newConf
	a.parent.apiConfigSet(newConf)

	ctx.Status(http.StatusOK)
}

func (a *api) onConfigPathsDelete(ctx *gin.Context) {
	name, ok := paramName(ctx)
	if !ok {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid name"))
		return
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	newConf := a.conf.Clone()

	err := newConf.RemovePath(name)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = newConf.Check()
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	a.conf = newConf
	a.parent.apiConfigSet(newConf)

	ctx.Status(http.StatusOK)
}

func (a *api) onPathsList(ctx *gin.Context) {
	data, err := a.pathManager.apiPathsList()
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

func (a *api) onPathsGet(ctx *gin.Context) {
	name, ok := paramName(ctx)
	if !ok {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid name"))
		return
	}

	data, err := a.pathManager.apiPathsGet(name)
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTSPConnsList(ctx *gin.Context) {
	data, err := a.rtspServer.APIConnsList()
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

func (a *api) onRTSPConnsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	data, err := a.rtspServer.APIConnsGet(uuid)
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTSPSessionsList(ctx *gin.Context) {
	data, err := a.rtspServer.APISessionsList()
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

func (a *api) onRTSPSessionsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	data, err := a.rtspServer.APISessionsGet(uuid)
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTSPSessionsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.rtspServer.APISessionsKick(uuid)
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *api) onRTSPSConnsList(ctx *gin.Context) {
	data, err := a.rtspsServer.APIConnsList()
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

func (a *api) onRTSPSConnsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	data, err := a.rtspsServer.APIConnsGet(uuid)
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTSPSSessionsList(ctx *gin.Context) {
	data, err := a.rtspsServer.APISessionsList()
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

func (a *api) onRTSPSSessionsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	data, err := a.rtspsServer.APISessionsGet(uuid)
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTSPSSessionsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.rtspsServer.APISessionsKick(uuid)
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *api) onRTMPConnsList(ctx *gin.Context) {
	data, err := a.rtmpServer.APIConnsList()
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

func (a *api) onRTMPConnsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	data, err := a.rtmpServer.APIConnsGet(uuid)
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTMPConnsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.rtmpServer.APIConnsKick(uuid)
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *api) onRTMPSConnsList(ctx *gin.Context) {
	data, err := a.rtmpsServer.APIConnsList()
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

func (a *api) onRTMPSConnsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	data, err := a.rtmpsServer.APIConnsGet(uuid)
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onRTMPSConnsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.rtmpsServer.APIConnsKick(uuid)
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *api) onHLSMuxersList(ctx *gin.Context) {
	data, err := a.hlsManager.APIMuxersList()
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

func (a *api) onHLSMuxersGet(ctx *gin.Context) {
	name, ok := paramName(ctx)
	if !ok {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid name"))
		return
	}

	data, err := a.hlsManager.APIMuxersGet(name)
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onWebRTCSessionsList(ctx *gin.Context) {
	data, err := a.webRTCServer.APISessionsList()
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

func (a *api) onWebRTCSessionsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	data, err := a.webRTCServer.APISessionsGet(uuid)
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onWebRTCSessionsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.webRTCServer.APISessionsKick(uuid)
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.Status(http.StatusOK)
}

func (a *api) onSRTConnsList(ctx *gin.Context) {
	data, err := a.srtServer.APIConnsList()
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

func (a *api) onSRTConnsGet(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	data, err := a.srtServer.APIConnsGet(uuid)
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *api) onSRTConnsKick(ctx *gin.Context) {
	uuid, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.srtServer.APIConnsKick(uuid)
	if err != nil {
		a.writeError(ctx, http.StatusInternalServerError, err)
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
