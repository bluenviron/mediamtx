package api //nolint:revive

import (
	"net/http"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
	"github.com/gin-gonic/gin"
)

func (a *API) onConfigPathDefaultsGet(ctx *gin.Context) {
	a.mutex.RLock()
	c := a.Conf
	a.mutex.RUnlock()

	ctx.JSON(http.StatusOK, c.PathDefaults)
}

func (a *API) onConfigPathDefaultsPatch(ctx *gin.Context) {
	var p conf.OptionalPath
	err := jsonwrapper.Decode(ctx.Request.Body, &p)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	newConf := a.Conf.Clone()

	newConf.PatchPathDefaults(&p)

	err = newConf.Validate(nil)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	a.Conf = newConf
	a.Parent.APIConfigSet(newConf)

	a.writeOK(ctx)
}
