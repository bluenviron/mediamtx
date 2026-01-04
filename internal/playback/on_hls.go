package playback

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/recordstore"
	"github.com/gin-gonic/gin"
)

func (s *Server) onHLS(ctx *gin.Context) {
	pathName := ctx.Query("path")

	if !s.doAuth(ctx, pathName) {
		return
	}

	pathConf, err := s.safeFindPathConf(pathName)
	if err != nil {
		s.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	var start *time.Time
	rawStart := ctx.Query("start")
	if rawStart != "" {
		var tmp time.Time
		tmp, err = time.Parse(time.RFC3339, rawStart)
		if err != nil {
			s.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid start: %w", err))
			return
		}
		start = &tmp
	}

	var end *time.Time
	rawEnd := ctx.Query("end")
	if rawEnd != "" {
		var tmp time.Time
		tmp, err = time.Parse(time.RFC3339, rawEnd)
		if err != nil {
			s.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid end: %w", err))
			return
		}
		end = &tmp
	}

	segments, err := recordstore.FindSegments(pathConf, pathName, start, end)
	if err != nil {
		if errors.Is(err, recordstore.ErrNoSegmentsFound) {
			s.writeError(ctx, http.StatusNotFound, err)
		} else {
			s.writeError(ctx, http.StatusBadRequest, err)
		}
		return
	}

	entries, err := parseAndConcatenate(pathConf.RecordFormat, segments)
	if err != nil {
		s.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	if start != nil {
		firstEntry := entries[0]

		// when start is placed in a gap between the first and second segment,
		// or when there's no second segment,
		// the first segment is erroneously included with a negative duration.
		// remove it.
		if firstEntry.Start.Add(time.Duration(firstEntry.Duration)).Before(*start) {
			entries = entries[1:]

			if len(entries) == 0 {
				s.writeError(ctx, http.StatusNotFound, recordstore.ErrNoSegmentsFound)
				return
			}
		} else if firstEntry.Start.Before(*start) {
			entries[0].Duration -= listEntryDuration(start.Sub(firstEntry.Start))
			entries[0].Start = *start
		}
	}

	if end != nil {
		lastEntry := entries[len(entries)-1]
		if lastEntry.Start.Add(time.Duration(lastEntry.Duration)).After(*end) {
			entries[len(entries)-1].Duration = listEntryDuration(end.Sub(lastEntry.Start))
		}
	}

	var scheme string
	if s.Encryption {
		scheme = "https"
	} else {
		scheme = "http"
	}

	// Generate HLS manifest
	var manifest strings.Builder
	manifest.WriteString("#EXTM3U\n")
	manifest.WriteString("#EXT-X-VERSION:3\n")
	manifest.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")

	// Find maximum duration for TARGETDURATION
	maxDuration := 0.0
	for _, entry := range entries {
		dur := time.Duration(entry.Duration).Seconds()
		if dur > maxDuration {
			maxDuration = dur
		}
	}
	// Round up to nearest integer
	targetDuration := max(int(maxDuration)+1, 1)
	manifest.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", targetDuration))
	manifest.WriteString("#EXT-X-MEDIA-SEQUENCE:0\n")

	// Add each segment
	for i, entry := range entries {
		duration := time.Duration(entry.Duration).Seconds()
		manifest.WriteString(fmt.Sprintf("#EXTINF:%.3f,\n", duration))

		// Generate segment URL with .m4s extension for HLS compatibility
		v := url.Values{}
		v.Add("path", pathName)
		v.Add("start", entry.Start.Format(time.RFC3339Nano))
		v.Add("duration", strconv.FormatFloat(duration, 'f', -1, 64))
		v.Add("format", "fmp4")
		u := &url.URL{
			Scheme:   scheme,
			Host:     ctx.Request.Host,
			Path:     fmt.Sprintf("/segment_%d.m4s", i),
			RawQuery: v.Encode(),
		}
		manifest.WriteString(u.String() + "\n")
	}

	manifest.WriteString("#EXT-X-ENDLIST\n")

	ctx.Header("Content-Type", "application/vnd.apple.mpegurl")
	ctx.String(http.StatusOK, manifest.String())
}

