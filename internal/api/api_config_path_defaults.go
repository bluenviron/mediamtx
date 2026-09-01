package api //nolint:revive

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

func (a *API) onConfigPathDefaultsGet(ctx *gin.Context) {
	c := conf.Redact(a.Parent.APIConfigSnapshot())

	ctx.JSON(http.StatusOK, c.PathDefaults)
}

func (a *API) onConfigPathDefaultsPatch(ctx *gin.Context) {
	if ctx.ContentType() != "application/json" {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("Content-Type must be application/json"))
		return
	}

	var p conf.OptionalPath
	err := jsonwrapper.Decode(&customLimitReader{ctx.Request.Body, maxInboundConfigSize}, &p)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.Parent.APIConfigPathDefaultsPatch(p)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	a.writeOK(ctx)
}
