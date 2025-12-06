//nolint:dupl
package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/bluenviron/mediamtx/internal/servers/hls"
	"github.com/gin-gonic/gin"
)

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
