package api //nolint:revive

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

func (a *API) onConfigGlobalGet(ctx *gin.Context) {
	c := conf.Redact(a.Parent.APIConfigSnapshot())

	ctx.JSON(http.StatusOK, c.Global())
}

func (a *API) onConfigGlobalPatch(ctx *gin.Context) {
	if ctx.ContentType() != "application/json" {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("Content-Type must be application/json"))
		return
	}

	var c conf.OptionalGlobal
	err := jsonwrapper.Decode(&customLimitReader{ctx.Request.Body, maxInboundConfigSize}, &c)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.Parent.APIConfigGlobalPatch(c)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	a.writeOK(ctx)
}
