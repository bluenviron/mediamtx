package api //nolint:revive,dupl

import (
	"errors"
	"net/http"

	"github.com/bluenviron/mediamtx/internal/servers/moq"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (a *API) onMoQSessionsList(ctx *gin.Context) {
	data, err := a.MoQServer.APISessionsList()
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

func (a *API) onMoQSessionsGet(ctx *gin.Context) {
	id, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	data, err := a.MoQServer.APISessionsGet(id)
	if err != nil {
		if errors.Is(err, moq.ErrSessionNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onMoQSessionsKick(ctx *gin.Context) {
	id, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.MoQServer.APISessionsKick(id)
	if err != nil {
		if errors.Is(err, moq.ErrSessionNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	a.writeOK(ctx)
}
