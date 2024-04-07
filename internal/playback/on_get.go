package playback

import (
	"errors"
	"fmt"
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
	segments []*Segment,
	start time.Time,
	duration time.Duration,
	m muxer,
) error {
	if recordFormat == conf.RecordFormatFMP4 {
		var firstInit []byte
		var segmentEnd time.Time

		err := func() error {
			f, err := os.Open(segments[0].Fpath)
			if err != nil {
				return err
			}
			defer f.Close()

			firstInit, err = segmentFMP4ReadInit(f)
			if err != nil {
				return err
			}

			m.writeInit(firstInit)

			segmentStartOffset := start.Sub(segments[0].Start)

			segmentMaxElapsed, err := segmentFMP4SeekAndMuxParts(f, segmentStartOffset, duration, m)
			if err != nil {
				return err
			}

			segmentEnd = start.Add(segmentMaxElapsed)

			return nil
		}()
		if err != nil {
			return err
		}

		for _, seg := range segments[1:] {
			err := func() error {
				f, err := os.Open(seg.Fpath)
				if err != nil {
					return err
				}
				defer f.Close()

				init, err := segmentFMP4ReadInit(f)
				if err != nil {
					return err
				}

				if !segmentFMP4CanBeConcatenated(firstInit, segmentEnd, init, seg.Start) {
					return errStopIteration
				}

				segmentStartOffset := seg.Start.Sub(start)

				segmentMaxElapsed, err := segmentFMP4WriteParts(f, segmentStartOffset, duration, m)
				if err != nil {
					return err
				}

				segmentEnd = start.Add(segmentMaxElapsed)

				return nil
			}()
			if err != nil {
				if errors.Is(err, errStopIteration) {
					return nil
				}

				return err
			}
		}

		err = m.finalFlush()
		if err != nil {
			return err
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
	sw := &muxerFMP4{w: ww}

	err = seekAndMux(pathConf.RecordFormat, segments, start, duration, sw)
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
