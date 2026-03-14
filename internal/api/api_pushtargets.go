//nolint:dupl
package api //nolint:revive

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/push"
)

func splitPushTargetPath(value string) (string, uuid.UUID, error) {
	i := strings.LastIndex(value, "/")
	if i < 0 {
		return "", uuid.UUID{}, fmt.Errorf("invalid path format, expected: /path/targetID")
	}

	id, err := uuid.Parse(value[i+1:])
	if err != nil {
		return "", uuid.UUID{}, fmt.Errorf("invalid target ID: %w", err)
	}

	return value[:i], id, nil
}

func pathToOptional(pathConf *conf.Path) (*conf.OptionalPath, error) {
	byts, err := json.Marshal(pathConf)
	if err != nil {
		return nil, err
	}

	var optional conf.OptionalPath
	err = json.Unmarshal(byts, &optional)
	if err != nil {
		return nil, err
	}

	return &optional, nil
}

func (a *API) persistPushTargets(runtimePathName string, mutate func(*conf.Path) error) error {
	if a.Conf == nil {
		return nil
	}

	pathData, err := a.PathManager.APIPathsGet(runtimePathName)
	if err != nil {
		return err
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	newConf := a.Conf.Clone()
	pathConf, ok := newConf.Paths[pathData.ConfName]
	if !ok {
		return conf.ErrPathNotFound
	}

	updated := pathConf.Clone()
	err = mutate(updated)
	if err != nil {
		return err
	}

	optional, err := pathToOptional(updated)
	if err != nil {
		return err
	}

	err = newConf.ReplacePath(pathData.ConfName, optional)
	if err != nil {
		return err
	}

	err = newConf.Validate(nil)
	if err != nil {
		return err
	}

	a.Conf = newConf
	a.Parent.APIConfigSet(newConf)

	return nil
}

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

	pathName, id, err := splitPushTargetPath(pathName)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	data, err := a.PathManager.APIPushTargetsGet(pathName, id)
	if err != nil {
		if errors.Is(err, conf.ErrPathNotFound) || errors.Is(err, push.ErrTargetNotFound) {
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

	err = a.persistPushTargets(pathName, func(pathConf *conf.Path) error {
		pathConf.PushTargets = append(pathConf.PushTargets, conf.PushTarget{URL: req.URL})
		return nil
	})
	if err != nil {
		_ = a.PathManager.APIPushTargetsRemove(pathName, data.ID)
		a.writeError(ctx, http.StatusInternalServerError, err)
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

	pathName, id, err := splitPushTargetPath(pathName)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	target, err := a.PathManager.APIPushTargetsGet(pathName, id)
	if err != nil {
		if errors.Is(err, conf.ErrPathNotFound) || errors.Is(err, push.ErrTargetNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	err = a.PathManager.APIPushTargetsRemove(pathName, id)
	if err != nil {
		if errors.Is(err, conf.ErrPathNotFound) || errors.Is(err, push.ErrTargetNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	err = a.persistPushTargets(pathName, func(pathConf *conf.Path) error {
		for i, persisted := range pathConf.PushTargets {
			if persisted.URL == target.URL {
				pathConf.PushTargets = append(pathConf.PushTargets[:i], pathConf.PushTargets[i+1:]...)
				return nil
			}
		}

		return push.ErrTargetNotFound
	})
	if err != nil {
		if errors.Is(err, push.ErrTargetNotFound) || errors.Is(err, conf.ErrPathNotFound) {
			a.writeError(ctx, http.StatusNotFound, err)
		} else {
			a.writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	a.writeOK(ctx)
}
