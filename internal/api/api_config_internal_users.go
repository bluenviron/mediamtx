package api //nolint:revive

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
	"github.com/bluenviron/mediamtx/internal/defs"
)

func (a *API) onConfigInternalUsersList(ctx *gin.Context) {
	data := &defs.APIInternalUserList{Items: a.Parent.APIConfigInternalUsersSnapshot()}
	data.ItemCount = len(data.Items)
	pageCount, err := paginate(&data.Items, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}
	data.PageCount = pageCount

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onConfigInternalUsersGet(ctx *gin.Context) {
	id, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	for _, item := range a.Parent.APIConfigInternalUsersSnapshot() {
		if item.ID == id {
			ctx.JSON(http.StatusOK, item)
			return
		}
	}

	a.writeError(ctx, http.StatusNotFound, conf.ErrAuthInternalUserNotFound)
}

func (a *API) onConfigInternalUsersAdd(ctx *gin.Context) {
	var u conf.AuthInternalUser
	err := jsonwrapper.Decode(&customLimitReader{ctx.Request.Body, maxInboundConfigSize}, &u)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	id, err := a.Parent.APIConfigInternalUsersAdd(a.ctx, u)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	ctx.JSON(http.StatusOK, &defs.APIInternalUserAddRes{Status: defs.APIOKStatusOK, ID: id})
}

func (a *API) onConfigInternalUsersPatch(ctx *gin.Context) {
	id, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	var u conf.OptionalAuthInternalUser
	err = jsonwrapper.Decode(&customLimitReader{ctx.Request.Body, maxInboundConfigSize}, &u)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.Parent.APIConfigInternalUsersPatch(a.ctx, id, u)
	if err != nil {
		if errors.Is(err, conf.ErrAuthInternalUserNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusBadRequest, err)
		}
		return
	}

	a.writeOK(ctx)
}

func (a *API) onConfigInternalUsersReplace(ctx *gin.Context) {
	id, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	var u conf.AuthInternalUser
	err = jsonwrapper.Decode(&customLimitReader{ctx.Request.Body, maxInboundConfigSize}, &u)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.Parent.APIConfigInternalUsersReplace(a.ctx, id, u)
	if err != nil {
		if errors.Is(err, conf.ErrAuthInternalUserNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusBadRequest, err)
		}
		return
	}

	a.writeOK(ctx)
}

func (a *API) onConfigInternalUsersDelete(ctx *gin.Context) {
	id, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	err = a.Parent.APIConfigInternalUsersDelete(a.ctx, id)
	if err != nil {
		if errors.Is(err, conf.ErrAuthInternalUserNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusBadRequest, err)
		}
		return
	}

	a.writeOK(ctx)
}
