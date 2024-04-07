package playback

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/gin-gonic/gin"
)

var errStopIteration = errors.New("stop iteration")

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
	segments []*Segment,
	start time.Time,
	duration time.Duration,
	w io.Writer,
) error {
	if recordFormat == conf.RecordFormatFMP4 {
		minTime := start.Sub(segments[0].Start)
		maxTime := minTime + duration
		var init []byte
		var maxElapsed time.Duration

		err := func() error {
			f, err := os.Open(segments[0].Fpath)
			if err != nil {
				return err
			}
			defer f.Close()

			init, err = fmp4ReadInit(f)
			if err != nil {
				return err
			}

			maxElapsed, err = fmp4SeekAndMuxParts(f, init, minTime, maxTime, w)
			if err != nil {
				return err
			}

			return nil
		}()
		if err != nil {
			return err
		}

		duration -= maxElapsed
		overallElapsed := maxElapsed
		prevInit := init
		prevEnd := start.Add(maxElapsed)

		for _, seg := range segments[1:] {
			err := func() error {
				f, err := os.Open(seg.Fpath)
				if err != nil {
					return err
				}
				defer f.Close()

				init, err := fmp4ReadInit(f)
				if err != nil {
					return err
				}

				if !fmp4CanBeConcatenated(prevInit, prevEnd, init, seg.Start) {
					return errStopIteration
				}

				maxElapsed, err = fmp4MuxParts(f, overallElapsed, duration, w)
				if err != nil {
					return err
				}

				return nil
			}()
			if err != nil {
				if errors.Is(err, errStopIteration) {
					return nil
				}

				return err
			}

			duration -= maxElapsed
			overallElapsed += maxElapsed
			prevEnd = seg.Start.Add(maxElapsed)
		}

		return nil
	}

	return fmt.Errorf("MPEG-TS format is not supported yet")
}

func (p *Server) onGet(ctx *gin.Context) {
	pathName := ctx.Query("path")

	if !p.doAuth(ctx, pathName) {
		return
	}

	start, err := time.Parse(time.RFC3339, ctx.Query("start"))
	if err != nil {
		p.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid start: %w", err))
		return
	}

	duration, err := parseDuration(ctx.Query("duration"))
	if err != nil {
		p.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid duration: %w", err))
		return
	}

	format := ctx.Query("format")
	if format != "" && format != "fmp4" {
		p.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid format: %s", format))
		return
	}

	pathConf, err := p.safeFindPathConf(pathName)
	if err != nil {
		p.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	segments, err := findSegmentsInTimespan(pathConf, pathName, start, duration)
	if err != nil {
		if errors.Is(err, errNoSegmentsFound) {
			p.writeError(ctx, http.StatusNotFound, err)
		} else {
			p.writeError(ctx, http.StatusBadRequest, err)
		}
		return
	}

	ww := &writerWrapper{ctx: ctx}

	err = seekAndMux(pathConf.RecordFormat, segments, start, duration, ww)
	if err != nil {
		// user aborted the download
		var neterr *net.OpError
		if errors.As(err, &neterr) {
			return
		}

		// nothing has been written yet; send back JSON
		if !ww.written {
			if errors.Is(err, errNoSegmentsFound) {
				p.writeError(ctx, http.StatusNotFound, err)
			} else {
				p.writeError(ctx, http.StatusBadRequest, err)
			}
			return
		}

		// something has already been written: abort and write logs only
		p.Log(logger.Error, err.Error())
		return
	}
}
