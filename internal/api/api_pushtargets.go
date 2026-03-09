//nolint:dupl
package api //nolint:revive

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
)

func (a *API) onPushTargetsList(ctx *gin.Context) {
	pathName, ok := paramName(ctx)
	if !ok {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid path name"))
		return
	}

	data, err := a.PathManager.APIPushTargetsList(pathName)
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

func (a *API) onPushTargetsGet(ctx *gin.Context) {
	pathName, ok := paramName(ctx)
	if !ok {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid path name"))
		return
	}

	// Extract ID from path: /pathName/targetID
	parts := strings.SplitN(pathName, "/", 2)
	if len(parts) != 2 {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid path format, expected: /path/targetID"))
		return
	}
	pathName = parts[0]

	id, err := uuid.Parse(parts[1])
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid target ID: %w", err))
		return
	}

	data, err := a.PathManager.APIPushTargetsGet(pathName, id)
	if err != nil {
		if errors.Is(err, conf.ErrPathNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onPushTargetsAdd(ctx *gin.Context) {
	pathName, ok := paramName(ctx)
	if !ok {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid path name"))
		return
	}

	var req defs.APIPushTargetAdd
	if err := ctx.ShouldBindJSON(&req); err != nil {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	if req.URL == "" {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("url is required"))
		return
	}

	data, err := a.PathManager.APIPushTargetsAdd(pathName, req)
	if err != nil {
		if errors.Is(err, conf.ErrPathNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onPushTargetsRemove(ctx *gin.Context) {
	pathName, ok := paramName(ctx)
	if !ok {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid path name"))
		return
	}

	// Extract ID from path: /pathName/targetID
	parts := strings.SplitN(pathName, "/", 2)
	if len(parts) != 2 {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid path format, expected: /path/targetID"))
		return
	}
	pathName = parts[0]

	id, err := uuid.Parse(parts[1])
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid target ID: %w", err))
		return
	}

	err = a.PathManager.APIPushTargetsRemove(pathName, id)
	if err != nil {
		if errors.Is(err, conf.ErrPathNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{})
}
