package api //nolint:revive

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/forward"
)

func (a *API) onForwardDestsList(ctx *gin.Context) {
	pathName := ctx.Query("path")
	if pathName == "" {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid path"))
		return
	}

	data, err := a.PathManager.APIForwardDestList(pathName)
	if err != nil {
		if errors.Is(err, conf.ErrPathNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
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

func (a *API) onForwardDestsGet(ctx *gin.Context) {
	id, err := uuid.Parse(ctx.Query("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	pathName := ctx.Query("path")
	if pathName == "" {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid path"))
		return
	}

	data, err := a.PathManager.APIForwardDestGet(pathName, id)
	if err != nil {
		if errors.Is(err, conf.ErrPathNotFound) || errors.Is(err, forward.ErrDestNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.JSON(http.StatusOK, data)
}
