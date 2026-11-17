package api //nolint:revive

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
)

func (a *API) onPathReadersList(ctx *gin.Context) {
	pathName := ctx.Query("path")
	if pathName == "" {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid path"))
		return
	}

	data, err := a.PathManager.APIReadersList(pathName)
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

func (a *API) onPathReadersGet(ctx *gin.Context) {
	pathName := ctx.Query("path")
	if pathName == "" {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid path"))
		return
	}

	id := ctx.Query("id")
	if id == "" {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return
	}

	data, err := a.PathManager.APIReadersGet(pathName, id)
	if err != nil {
		if errors.Is(err, conf.ErrPathNotFound) || errors.Is(err, defs.ErrPathReaderNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.JSON(http.StatusOK, data)
}
