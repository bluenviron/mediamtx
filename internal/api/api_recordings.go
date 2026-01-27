package api //nolint:revive

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/recordstore"
	"github.com/gin-gonic/gin"
)

func (a *API) onRecordingsList(ctx *gin.Context) {
	a.mutex.RLock()
	c := a.Conf
	a.mutex.RUnlock()

	pathNames := recordstore.FindAllPathsWithSegments(c.Paths)

	data := defs.APIRecordingList{}

	data.ItemCount = len(pathNames)
	pageCount, err := paginate(&pathNames, ctx.Query("itemsPerPage"), ctx.Query("page"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}
	data.PageCount = pageCount

	data.Items = make([]defs.APIRecording, len(pathNames))

	for i, pathName := range pathNames {
		pathConf, _, _ := conf.FindPathConf(c.Paths, pathName)
		data.Items[i] = *recordingsOfPath(pathConf, pathName)
	}

	ctx.JSON(http.StatusOK, data)
}

func (a *API) onRecordingsGet(ctx *gin.Context) {
	pathName, ok := paramName(ctx)
	if !ok {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid name"))
		return
	}

	a.mutex.RLock()
	c := a.Conf
	a.mutex.RUnlock()

	pathConf, _, err := conf.FindPathConf(c.Paths, pathName)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	ctx.JSON(http.StatusOK, recordingsOfPath(pathConf, pathName))
}

func (a *API) onRecordingDeleteSegment(ctx *gin.Context) {
	pathName := ctx.Query("path")

	start, err := time.Parse(time.RFC3339, ctx.Query("start"))
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid 'start' parameter: %w", err))
		return
	}

	a.mutex.RLock()
	c := a.Conf
	a.mutex.RUnlock()

	pathConf, _, err := conf.FindPathConf(c.Paths, pathName)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	pathFormat := recordstore.PathAddExtension(
		strings.ReplaceAll(pathConf.RecordPath, "%path", pathName),
		pathConf.RecordFormat,
	)

	segmentPath := recordstore.Path{
		Start: start,
	}.Encode(pathFormat)

	err = os.Remove(segmentPath)
	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	a.writeOK(ctx)
}
