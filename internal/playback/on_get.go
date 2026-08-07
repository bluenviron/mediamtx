package playback

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/recordstore"
)

type writerWrapper struct {
	ctx     *gin.Context
	written bool
}

func (w *writerWrapper) Write(p []byte) (int, error) {
	if !w.written {
		w.written = true
		w.ctx.Header("Accept-Ranges", "none")
		w.ctx.Header("Content-Type", "video/mp4")
	}
	return w.ctx.Writer.Write(p)
}

func parseDuration(raw string) (time.Duration, error) {
	// seconds
	if secs, err := strconv.ParseFloat(raw, 64); err == nil {
		return time.Duration(secs * float64(time.Second)), nil
	}

	// deprecated, golang format
	return time.ParseDuration(raw)
}

func seekAndMux(
	recordFormat conf.RecordFormat,
	segments []*recordstore.Segment,
	start time.Time,
	duration time.Duration,
	m muxer,
) error {
	if recordFormat == conf.RecordFormatFMP4 {
		f, err := os.Open(segments[0].Fpath)
		if err != nil {
			return err
		}
		defer f.Close()

		firstInit, _, err := segmentFMP4ReadHeader(f)
		if err != nil {
			return err
		}

		m.writeInit(&fmp4.Init{
			Tracks: firstInit.Tracks,
		})

		firstMtxi := findMtxi(firstInit.UserData)
		startOffset := segments[0].Start.Sub(start) // this is negative
		dts := startOffset
		prevInit := firstInit

		segmentDuration, err := segmentFMP4MuxParts(f, dts, duration, firstInit.Tracks, m)
		if err != nil {
			return err
		}

		segmentEnd := segments[0].Start.Add(segmentDuration)

		for _, seg := range segments[1:] {
			f, err = os.Open(seg.Fpath)
			if err != nil {
				return err
			}
			defer f.Close()

			var init *fmp4.Init
			init, _, err = segmentFMP4ReadHeader(f)
			if err != nil {
				return err
			}

			if !segmentFMP4CanBeConcatenated(prevInit, segmentEnd, init, seg.Start) {
				break
			}

			if firstMtxi != nil {
				mtxi := findMtxi(init.UserData)
				dts = time.Duration(mtxi.DTS-firstMtxi.DTS) + startOffset
			} else { // legacy method
				dts = seg.Start.Sub(start) // this is positive
			}

			segmentDuration, err = segmentFMP4MuxParts(f, dts, duration, firstInit.Tracks, m)
			if err != nil {
				return err
			}

			segmentEnd = seg.Start.Add(segmentDuration)
			prevInit = init
		}

		err = m.flush()
		if err != nil {
			return err
		}

		return nil
	}

	return fmt.Errorf("MPEG-TS format is not supported yet")
}

func (s *Server) onGet(ctx *gin.Context) {
	pathName := ctx.Query("path")

	// validate path name before passing it to the authentication manager
	err := conf.IsValidPathName(pathName)
	if err != nil {
		s.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid path name: %w (%s)", err, pathName))
		return
	}

	if !s.doAuth(ctx, pathName) {
		return
	}

	start, err := time.Parse(time.RFC3339, ctx.Query("start"))
	if err != nil {
		s.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid start: %w", err))
		return
	}

	duration, err := parseDuration(ctx.Query("duration"))
	if err != nil {
		s.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid duration: %w", err))
		return
	}

	// origin numbers the output timeline from a moment of the caller's
	// choosing instead of from start. It exists so that consecutive requests
	// over one recording can be served as segments of a single stream: without
	// it every response begins at zero and a player stacks them on top of one
	// another rather than laying them end to end.
	//
	// It shifts the timeline only. Which samples are returned, and which of
	// them are visible rather than decode-only, still follow start.
	origin := start
	if rawOrigin := ctx.Query("origin"); rawOrigin != "" {
		origin, err = time.Parse(time.RFC3339, rawOrigin)
		if err != nil {
			s.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid origin: %w", err))
			return
		}
		if origin.After(start) {
			s.writeError(ctx, http.StatusBadRequest,
				fmt.Errorf("origin is after start"))
			return
		}
	}

	ww := &writerWrapper{ctx: ctx}
	var m muxer

	format := ctx.Query("format")
	switch format {
	case "", "fmp4":
		m = &muxerFMP4{w: ww, baseOffset: start.Sub(origin)}

	case "mp4":
		m = &muxerMP4{w: ww}

	default:
		s.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid format: %s", format))
		return
	}

	pathConf, err := s.safeFindPathConf(pathName)
	if err != nil {
		s.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	end := start.Add(duration)
	segments, err := recordstore.FindSegments(pathConf, pathName, &start, &end)
	if err != nil {
		if errors.Is(err, recordstore.ErrNoSegmentsFound) {
			s.writeError(ctx, http.StatusNotFound, err)
		} else {
			s.writeError(ctx, http.StatusBadRequest, err)
		}
		return
	}

	err = seekAndMux(pathConf.RecordFormat, segments, start, duration, m)
	if err != nil {
		// user aborted the download
		if _, ok := errors.AsType[*net.OpError](err); ok {
			return
		}

		// nothing has been written yet; send back JSON
		if !ww.written {
			if errors.Is(err, recordstore.ErrNoSegmentsFound) {
				s.writeError(ctx, http.StatusNotFound, err)
			} else {
				s.writeError(ctx, http.StatusBadRequest, err)
			}
			return
		}

		// something has already been written: abort and write logs only
		s.Log(logger.Error, err.Error())
		return
	}
}
