package api //nolint:revive

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/playback"
	"github.com/bluenviron/mediamtx/internal/recordstore"
	"github.com/gin-gonic/gin"
)

type APIRecordingSession struct {
	Start    string  `json:"start"`
	Duration float64 `json:"duration"`
	URL      string  `json:"url"`
}

type recordingSegmentInfo struct {
	start time.Time
	end   time.Time
}

func recordingsSessionsOfPath(
	pathConf *conf.Path,
	pathName string,
	start *time.Time,
	end *time.Time,
) gin.H {

	segments, _ := recordstore.FindSegments(
		pathConf,
		pathName,
		nil,
		nil,
	)

	if len(segments) == 0 {
		return gin.H{
			"name":     pathName,
			"sessions": []APIRecordingSession{},
		}
	}

	infos := make(
		[]recordingSegmentInfo,
		0,
		len(segments),
	)

	for _, seg := range segments {

		duration, err := playback.GetSegmentDuration(
			seg.Fpath,
		)

		if err != nil {
			continue
		}

		segStart := seg.Start
		segEnd := seg.Start.Add(duration)

		if start != nil && segEnd.Before(*start) {
			continue
		}

		if end != nil && segStart.After(*end) {
			continue
		}

		infos = append(
			infos,
			recordingSegmentInfo{
				start: segStart,
				end:   segEnd,
			},
		)
	}

	if len(infos) == 0 {
		return gin.H{
			"name":     pathName,
			"sessions": []APIRecordingSession{},
		}
	}

	sessions := []APIRecordingSession{}

	sessionStart := infos[0].start
	sessionEnd := infos[0].end

	for i := 1; i < len(infos); i++ {

		gap := infos[i].start.Sub(sessionEnd)

		if gap > time.Second {

			sessions = append(
				sessions,
				makeRecordingSession(
					pathName,
					sessionStart,
					sessionEnd,
				),
			)

			sessionStart = infos[i].start
			sessionEnd = infos[i].end

			continue
		}

		if infos[i].end.After(sessionEnd) {
			sessionEnd = infos[i].end
		}
	}

	sessions = append(
		sessions,
		makeRecordingSession(
			pathName,
			sessionStart,
			sessionEnd,
		),
	)

	return gin.H{
		"name":     pathName,
		"sessions": sessions,
	}
}

func makeRecordingSession(
	pathName string,
	start time.Time,
	end time.Time,
) APIRecordingSession {

	duration := end.Sub(start).Seconds()

	return APIRecordingSession{
		Start: start.Format(time.RFC3339Nano),

		Duration: duration,

		URL: fmt.Sprintf(
			"http://localhost:9996/get?duration=%f&path=%s&start=%s",
			duration,
			pathName,
			url.QueryEscape(
				start.Format(time.RFC3339Nano),
			),
		),
	}
}

func recordingsOfPath(
	pathConf *conf.Path,
	pathName string,
) *defs.APIRecording {
	ret := &defs.APIRecording{
		Name: pathName,
	}

	segments, _ := recordstore.FindSegments(pathConf, pathName, nil, nil)

	ret.Segments = make([]defs.APIRecordingSegment, len(segments))

	for i, seg := range segments {
		ret.Segments[i] = defs.APIRecordingSegment{
			Start: seg.Start,
		}
	}

	return ret
}

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

	var startPtr *time.Time
	var endPtr *time.Time

	if v := ctx.Query("start"); v != "" {
			t, err := time.Parse(time.RFC3339Nano, v)
			if err != nil {
					a.writeError(
							ctx,
							http.StatusBadRequest,
							fmt.Errorf("invalid start parameter: %w", err),
					)
					return
			}
			startPtr = &t
	}

	if v := ctx.Query("end"); v != "" {
			t, err := time.Parse(time.RFC3339Nano, v)
			if err != nil {
					a.writeError(
							ctx,
							http.StatusBadRequest,
							fmt.Errorf("invalid end parameter: %w", err),
					)
					return
			}
			endPtr = &t
	} else {
			now := time.Now()
			endPtr = &now
	}

	if ctx.Query("sessions") == "true" {
		ctx.JSON(
			http.StatusOK,
			recordingsSessionsOfPath(pathConf, pathName, startPtr, endPtr),
		)
		return
	}

	ctx.JSON(
		http.StatusOK,
		recordingsOfPath(pathConf, pathName),
	)
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

func (a *API) onRecordingDeleteSession(ctx *gin.Context) {

	pathName := ctx.Query("path")

	start, err := time.Parse(
		time.RFC3339Nano,
		ctx.Query("start"),
	)

	if err != nil {
		a.writeError(
			ctx,
			http.StatusBadRequest,
			fmt.Errorf("invalid 'start' parameter: %w", err),
		)
		return
	}

	a.mutex.RLock()
	c := a.Conf
	a.mutex.RUnlock()

	pathConf, _, err := conf.FindPathConf(
		c.Paths,
		pathName,
	)

	if err != nil {
		a.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	segments, _ := recordstore.FindSegments(
		pathConf,
		pathName,
		nil,
		nil,
	)

	type sessionSegment struct {
		path  string
		start time.Time
		end   time.Time
	}

	var sessions [][]sessionSegment

	var current []sessionSegment

	var lastEnd time.Time

	for _, seg := range segments {
		duration, err := playback.GetSegmentDuration(
			seg.Fpath,
		)

		if err != nil {
			continue
		}

		end := seg.Start.Add(duration)

		if len(current) == 0 {
			current = append(
				current,
				sessionSegment{
					path:  seg.Fpath,
					start: seg.Start,
					end:   end,
				},
			)
			lastEnd = end
			continue
		}

		gap := seg.Start.Sub(lastEnd)

		if gap > time.Second {
			sessions = append(
				sessions,
				current,
			)
			current = []sessionSegment{}
		}
		current = append(
			current,
			sessionSegment{
				path:  seg.Fpath,
				start: seg.Start,
				end:   end,
			},
		)
		if end.After(lastEnd) {
			lastEnd = end
		}
	}

	if len(current) > 0 {
		sessions = append(
			sessions,
			current,
		)
	}

	found := false

	for _, session := range sessions {
		if len(session) == 0 {
			continue
		}
		sessionStart := session[0].start

		if sessionStart.Equal(start) {
			found = true
			for _, seg := range session {

				_ = os.Remove(seg.path)
			}
			break
		}
	}

	if !found {
		a.writeError(
			ctx,
			http.StatusBadRequest,
			fmt.Errorf("session not found"),
		)
		return
	}

	a.writeOK(ctx)
}
