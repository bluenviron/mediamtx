package api //nolint:revive

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
	"github.com/bluenviron/mediamtx/internal/defs"
)

func (a *API) onConfigPathsList(ctx *gin.Context) {
	c := conf.Redact(a.Parent.APIConfigSnapshot())

	data := &defs.APIPathConfList{
		Items: make([]conf.Path, len(c.Paths)),
	}

	for i, key := range sortedKeys(c.Paths) {
		data.Items[i] = *c.Paths[key]
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

func (a *API) onConfigPathsGet(ctx *gin.Context) {
	confName, ok := paramName(ctx)
	if !ok {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid name"))
		return
	}

	c := conf.Redact(a.Parent.APIConfigSnapshot())

	p, ok := c.Paths[confName]
	if !ok {
		a.writeError(ctx, http.StatusNotFound, fmt.Errorf("path configuration not found"))
		return
	}

	ctx.JSON(http.StatusOK, p)
}

func (a *API) onConfigPathsAdd(ctx *gin.Context) { //nolint:dupl
	if ctx.ContentType() != "application/json" {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("Content-Type must be application/json"))
		return
	}

	confName, ok := paramName(ctx)
	if !ok {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid name"))
		return
	}

	var p conf.OptionalPath
	err := jsonwrapper.Decode(&customLimitReader{ctx.Request.Body, maxInboundConfigSize}, &p)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.Parent.APIConfigPathsAdd(confName, p)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	a.writeOK(ctx)
}

func (a *API) onConfigPathsPatch(ctx *gin.Context) { //nolint:dupl
	if ctx.ContentType() != "application/json" {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("Content-Type must be application/json"))
		return
	}

	confName, ok := paramName(ctx)
	if !ok {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid name"))
		return
	}

	var p conf.OptionalPath
	err := jsonwrapper.Decode(&customLimitReader{ctx.Request.Body, maxInboundConfigSize}, &p)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.Parent.APIConfigPathsPatch(confName, p)
	if err != nil {
		if errors.Is(err, conf.ErrPathNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusBadRequest, err)
		}
		return
	}

	a.writeOK(ctx)
}

func (a *API) onConfigPathsReplace(ctx *gin.Context) { //nolint:dupl
	if ctx.ContentType() != "application/json" {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("Content-Type must be application/json"))
		return
	}

	confName, ok := paramName(ctx)
	if !ok {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid name"))
		return
	}

	var p conf.OptionalPath
	err := jsonwrapper.Decode(&customLimitReader{ctx.Request.Body, maxInboundConfigSize}, &p)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.Parent.APIConfigPathsReplace(confName, p)
	if err != nil {
		if errors.Is(err, conf.ErrPathNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusBadRequest, err)
		}
		return
	}

	a.writeOK(ctx)
}

func (a *API) onConfigPathsDelete(ctx *gin.Context) {
	confName, ok := paramName(ctx)
	if !ok {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid name"))
		return
	}

	err := a.Parent.APIConfigPathsDelete(confName)
	if err != nil {
		if errors.Is(err, conf.ErrPathNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusBadRequest, err)
		}
		return
	}

	a.writeOK(ctx)
}
