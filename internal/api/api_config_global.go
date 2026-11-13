package api //nolint:revive

import (
	"net/http"

	"github.com/bluenviron/mediacommon/v2/pkg/limitedreader"
	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

func (a *API) onConfigGlobalGet(ctx *gin.Context) {
	c := conf.Redact(a.Parent.APIConfigSnapshot())

	ctx.JSON(http.StatusOK, c.Global())
}

func (a *API) onConfigGlobalPatch(ctx *gin.Context) {
	var c conf.OptionalGlobal
	err := jsonwrapper.Decode(&limitedreader.Reader{R: ctx.Request.Body, N: maxInboundConfigSize}, &c)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.Parent.APIConfigGlobalPatch(a.ctx, c)
	if err != nil {
		a.writeConfigWriteError(ctx, err)
		return
	}

	a.writeOK(ctx)
}
