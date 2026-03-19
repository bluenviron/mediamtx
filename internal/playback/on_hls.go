package playback

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/recordstore"
	"github.com/gin-gonic/gin"
)

// subdivideForPlayback splits large record segments into smaller HLS segments
// when maxDuration > 0. This improves playback startup since the player can
// begin after fetching the first small segment instead of a 30-minute file.
func subdivideForPlayback(parsed []*parsedSegment, maxDuration time.Duration) []*parsedSegment {
	if maxDuration <= 0 {
		return parsed
	}
	var flat []*parsedSegment
	for _, seg := range parsed {
		if seg.duration <= maxDuration {
			flat = append(flat, seg)
			continue
		}
		elapsed := time.Duration(0)
		for elapsed < seg.duration {
			d := min(seg.duration-elapsed, maxDuration)
			flat = append(flat, &parsedSegment{
				start:    seg.start.Add(elapsed),
				init:     seg.init,
				duration: d,
			})
			elapsed += d
		}
	}
	return flat
}

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

	if pathConf.RecordFormat != conf.RecordFormatFMP4 {
		s.writeError(ctx, http.StatusBadRequest, fmt.Errorf("HLS playback requires fMP4 recording format"))
		return
	}

	// Parse each recording file individually (don't concatenate) so each
	// recording file becomes its own HLS segment for faster startup.
	parsed, err := parseSegments(segments)
	if err != nil {
		s.writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	// Filter by start/end time
	if start != nil {
		for len(parsed) > 0 {
			seg := parsed[0]
			segEnd := seg.start.Add(seg.duration)
			if segEnd.Before(*start) {
				parsed = parsed[1:]
			} else {
				break
			}
		}
	}
	if end != nil {
		for len(parsed) > 0 {
			seg := parsed[len(parsed)-1]
			if seg.start.After(*end) {
				parsed = parsed[:len(parsed)-1]
			} else {
				break
			}
		}
	}

	if len(parsed) == 0 {
		s.writeError(ctx, http.StatusNotFound, recordstore.ErrNoSegmentsFound)
		return
	}

	// Subdivide large record segments into smaller HLS segments for faster startup.
	// When playbackSegmentDuration > 0, segments longer than that are split into
	// multiple manifest entries. Each sub-segment is served via /get with start+duration.
	flat := subdivideForPlayback(parsed, time.Duration(pathConf.PlaybackSegmentDuration))

	// Generate HLS manifest
	var manifest strings.Builder
	manifest.WriteString("#EXTM3U\n")
	manifest.WriteString("#EXT-X-VERSION:6\n")
	manifest.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")

	// Find maximum duration for TARGETDURATION
	maxDuration := 0.0
	for _, seg := range flat {
		dur := seg.duration.Seconds()
		if dur > maxDuration {
			maxDuration = dur
		}
	}
	targetDuration := max(int(maxDuration)+1, 1)
	manifest.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", targetDuration))
	manifest.WriteString("#EXT-X-MEDIA-SEQUENCE:0\n")

	// EXT-X-MAP tells the player where to get the init segment (ftyp+moov).
	// Without this, fMP4 segments (moof+mdat) cannot be decoded.
	initParams := url.Values{}
	initParams.Add("path", pathName)
	manifest.WriteString(fmt.Sprintf("#EXT-X-MAP:URI=\"hls_init.mp4?%s\"\n", initParams.Encode()))

	// Add each segment (possibly subdivided from record files).
	// skipInit=true tells the segment endpoint to omit the init data
	// since EXT-X-MAP already provides it.
	// timeOffset ensures each segment's baseMediaDecodeTime continues
	// where the previous segment ended (required for MSE).
	cumulativeOffset := 0.0
	for i, seg := range flat {
		duration := seg.duration.Seconds()
		manifest.WriteString(fmt.Sprintf("#EXT-X-PROGRAM-DATE-TIME:%s\n", seg.start.Format(time.RFC3339Nano)))
		manifest.WriteString(fmt.Sprintf("#EXTINF:%.3f,\n", duration))

		v := url.Values{}
		v.Add("path", pathName)
		v.Add("start", seg.start.Format(time.RFC3339Nano))
		v.Add("duration", strconv.FormatFloat(duration, 'f', -1, 64))
		v.Add("format", "fmp4")
		v.Add("skipInit", "true")
		v.Add("timeOffset", strconv.FormatFloat(cumulativeOffset, 'f', -1, 64))
		manifest.WriteString(fmt.Sprintf("segment_%d.m4s?%s\n", i, v.Encode()))

		cumulativeOffset += duration
	}

	manifest.WriteString("#EXT-X-ENDLIST\n")

	ctx.Header("Content-Type", "application/vnd.apple.mpegurl")
	ctx.String(http.StatusOK, manifest.String())
}
