package api //nolint:revive

import (
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
)

func (a *API) onKeepaliveAdd(ctx *gin.Context) {
	pathName, ok := paramName(ctx)
	if !ok {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid name"))
		return
	}

	// create access request with authentication info from API request
	accessRequest := defs.PathAccessRequest{
		Name:        pathName,
		Query:       ctx.Request.URL.RawQuery,
		Publish:     false, // keepalive is a reader
		SkipAuth:    false,
		Proto:       "", // API-originated requests don't have a specific streaming protocol
		Credentials: httpp.Credentials(ctx.Request),
		IP:          net.ParseIP(ctx.ClientIP()),
	}

	id, err := a.PathManager.APIKeepaliveAdd(accessRequest)
	if err != nil {
		// check if it's an auth error
		var authErr *auth.Error
		if errors.As(err, &authErr) {
			a.writeError(ctx, http.StatusUnauthorized, err)
			return
		}
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	// return the keepalive ID in the response
	ctx.JSON(http.StatusOK, gin.H{"id": id})
}

func (a *API) onKeepaliveRemove(ctx *gin.Context) {
	id, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid keepalive ID"))
		return
	}

	// create access request with authentication info from API request
	accessRequest := defs.PathAccessRequest{
		Name:        "", // not needed for removal
		Query:       ctx.Request.URL.RawQuery,
		Publish:     false,
		SkipAuth:    false,
		Proto:       "",
		Credentials: httpp.Credentials(ctx.Request),
		IP:          net.ParseIP(ctx.ClientIP()),
	}

	err = a.PathManager.APIKeepaliveRemove(id, accessRequest)
	if err != nil {
		// check for auth/permission errors
		if errors.Is(err, conf.ErrPathNotFound) || err.Error() == "keepalive not found" {
			a.writeError(ctx, http.StatusNotFound, err)
			return
		}
		if err.Error() == "only the creator can remove this keepalive" {
			a.writeError(ctx, http.StatusForbidden, err)
			return
		}
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	a.writeOK(ctx)
}

func (a *API) onKeepalivesList(ctx *gin.Context) {
	data, err := a.PathManager.APIKeepalivesList()
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

func (a *API) onKeepalivesGet(ctx *gin.Context) {
	id, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid keepalive ID"))
		return
	}

	data, err := a.PathManager.APIKeepalivesGet(id)
	if err != nil {
		if err.Error() == "keepalive not found" {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.JSON(http.StatusOK, data)
}
