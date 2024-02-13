// Package playback contains the playback server.
package playback

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
	"github.com/bluenviron/mediamtx/internal/restrictnetwork"
	"github.com/gin-gonic/gin"
)

const (
	concatenationTolerance = 1 * time.Second
)

var errNoSegmentsFound = errors.New("no recording segments found for the given timestamp")

func parseDuration(raw string) (time.Duration, error) {
	// seconds
	if secs, err := strconv.ParseFloat(raw, 64); err == nil {
		return time.Duration(secs * float64(time.Second)), nil
	}

	// deprecated, golang format
	return time.ParseDuration(raw)
}

type listEntry struct {
	Start    time.Time `json:"start"`
	Duration float64   `json:"duration"`
}

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

// Server is the playback server.
type Server struct {
	Address     string
	ReadTimeout conf.StringDuration
	PathConfs   map[string]*conf.Path
	Parent      logger.Writer

	httpServer *httpp.WrappedServer
	mutex      sync.RWMutex
}

// Initialize initializes API.
func (p *Server) Initialize() error {
	router := gin.New()
	router.SetTrustedProxies(nil) //nolint:errcheck

	group := router.Group("/")

	group.GET("/list", p.onList)
	group.GET("/get", p.onGet)

	network, address := restrictnetwork.Restrict("tcp", p.Address)

	var err error
	p.httpServer, err = httpp.NewWrappedServer(
		network,
		address,
		time.Duration(p.ReadTimeout),
		"",
		"",
		router,
		p,
	)
	if err != nil {
		return err
	}

	p.Log(logger.Info, "listener opened on "+address)

	return nil
}

// Close closes Server.
func (p *Server) Close() {
	p.Log(logger.Info, "listener is closing")
	p.httpServer.Close()
}

// Log implements logger.Writer.
func (p *Server) Log(level logger.Level, format string, args ...interface{}) {
	p.Parent.Log(level, "[playback] "+format, args...)
}

// ReloadPathConfs is called by core.Core.
func (p *Server) ReloadPathConfs(pathConfs map[string]*conf.Path) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.PathConfs = pathConfs
}

func (p *Server) writeError(ctx *gin.Context, status int, err error) {
	// show error in logs
	p.Log(logger.Error, err.Error())

	// add error to response
	ctx.String(status, err.Error())
}

func (p *Server) safeFindPathConf(name string) (*conf.Path, error) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	_, pathConf, _, err := conf.FindPathConf(p.PathConfs, name)
	return pathConf, err
}

func (p *Server) onList(ctx *gin.Context) {
	pathName := ctx.Query("path")

	pathConf, err := p.safeFindPathConf(pathName)
	if err != nil {
		p.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	if !pathConf.Playback {
		p.writeError(ctx, http.StatusBadRequest, fmt.Errorf("playback is disabled on path '%s'", pathName))
		return
	}

	segments, err := FindSegments(pathConf, pathName)
	if err != nil {
		if errors.Is(err, errNoSegmentsFound) {
			p.writeError(ctx, http.StatusNotFound, err)
		} else {
			p.writeError(ctx, http.StatusBadRequest, err)
		}
		return
	}

	if pathConf.RecordFormat != conf.RecordFormatFMP4 {
		p.writeError(ctx, http.StatusBadRequest, fmt.Errorf("format of recording segments is not fmp4"))
		return
	}

	for _, seg := range segments {
		d, err := fmp4Duration(seg.fpath)
		if err != nil {
			p.writeError(ctx, http.StatusInternalServerError, err)
			return
		}
		seg.duration = d
	}

	segments = mergeConcatenatedSegments(segments)

	out := make([]listEntry, len(segments))
	for i, seg := range segments {
		out[i] = listEntry{
			Start:    seg.Start,
			Duration: seg.duration.Seconds(),
		}
	}

	ctx.JSON(http.StatusOK, out)
}

func (p *Server) onGet(ctx *gin.Context) {
	pathName := ctx.Query("path")

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

	if pathConf.RecordFormat != conf.RecordFormatFMP4 {
		p.writeError(ctx, http.StatusBadRequest, fmt.Errorf("format of recording segments is not fmp4"))
		return
	}

	ww := &writerWrapper{ctx: ctx}
	minTime := start.Sub(segments[0].Start)
	maxTime := minTime + duration

	elapsed, err := fmp4SeekAndMux(
		segments[0].fpath,
		minTime,
		maxTime,
		ww)
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

	start = start.Add(elapsed)
	duration -= elapsed
	overallElapsed := elapsed

	for _, seg := range segments[1:] {
		// there's a gap between segments, stop serving the recording
		if seg.Start.Before(start.Add(-concatenationTolerance)) || seg.Start.After(start.Add(concatenationTolerance)) {
			return
		}

		elapsed, err := fmp4Mux(seg.fpath, overallElapsed, duration, ctx.Writer)
		if err != nil {
			// user aborted the download
			var neterr *net.OpError
			if errors.As(err, &neterr) {
				return
			}

			// something has been already written: abort and write to logs only
			p.Log(logger.Error, err.Error())
			return
		}

		start = seg.Start.Add(elapsed)
		duration -= elapsed
		overallElapsed += elapsed
	}
}
