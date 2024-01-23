// Package playback contains the playback server.
package playback

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpserv"
	"github.com/bluenviron/mediamtx/internal/record"
	"github.com/bluenviron/mediamtx/internal/restrictnetwork"
	"github.com/gin-gonic/gin"
)

const (
	concatenationTolerance = 1 * time.Second
)

var errNoSegmentsFound = errors.New("no recording segments found for the given timestamp")

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

type segment struct {
	fpath string
	start time.Time
}

func findSegments(
	pathConf *conf.Path,
	pathName string,
	start time.Time,
	duration time.Duration,
) ([]segment, error) {
	if !pathConf.Playback {
		return nil, fmt.Errorf("playback is disabled on path '%s'", pathName)
	}

	recordPath := record.PathAddExtension(
		strings.ReplaceAll(pathConf.RecordPath, "%path", pathName),
		pathConf.RecordFormat,
	)

	// we have to convert to absolute paths
	// otherwise, recordPath and fpath inside Walk() won't have common elements
	recordPath, _ = filepath.Abs(recordPath)

	commonPath := record.CommonPath(recordPath)
	end := start.Add(duration)
	var segments []segment

	// gather all segments that starts before the end of the playback
	err := filepath.Walk(commonPath, func(fpath string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			var pa record.Path
			ok := pa.Decode(recordPath, fpath)
			if ok && !end.Before(time.Time(pa)) {
				segments = append(segments, segment{
					fpath: fpath,
					start: time.Time(pa),
				})
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	if segments == nil {
		return nil, errNoSegmentsFound
	}

	sort.Slice(segments, func(i, j int) bool {
		return segments[i].start.Before(segments[j].start)
	})

	// find the segment that may contain the start of the playback and remove all previous ones
	found := false
	for i := 0; i < len(segments)-1; i++ {
		if !start.Before(segments[i].start) && start.Before(segments[i+1].start) {
			segments = segments[i:]
			found = true
			break
		}
	}

	// otherwise, keep the last segment only and check whether it may contain the start of the playback
	if !found {
		segments = segments[len(segments)-1:]
		if segments[len(segments)-1].start.After(start) {
			return nil, errNoSegmentsFound
		}
	}

	return segments, nil
}

// Server is the playback server.
type Server struct {
	Address     string
	ReadTimeout conf.StringDuration
	PathConfs   map[string]*conf.Path
	Parent      logger.Writer

	httpServer *httpserv.WrappedServer
	mutex      sync.RWMutex
}

// Initialize initializes API.
func (p *Server) Initialize() error {
	router := gin.New()
	router.SetTrustedProxies(nil) //nolint:errcheck

	group := router.Group("/")

	group.GET("/get", p.onGet)

	network, address := restrictnetwork.Restrict("tcp", p.Address)

	var err error
	p.httpServer, err = httpserv.NewWrappedServer(
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

func (p *Server) onGet(ctx *gin.Context) {
	pathName := ctx.Query("path")

	start, err := time.Parse(time.RFC3339, ctx.Query("start"))
	if err != nil {
		p.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid start: %w", err))
		return
	}

	duration, err := time.ParseDuration(ctx.Query("duration"))
	if err != nil {
		p.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid duration: %w", err))
		return
	}

	format := ctx.Query("format")
	if format != "fmp4" {
		p.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid format: %s", format))
		return
	}

	pathConf, err := p.safeFindPathConf(pathName)
	if err != nil {
		p.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	segments, err := findSegments(pathConf, pathName, start, duration)
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
	minTime := start.Sub(segments[0].start)
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

		// something has been already written: abort and write to logs only
		p.Log(logger.Error, err.Error())
		return
	}

	start = start.Add(elapsed)
	duration -= elapsed
	overallElapsed := elapsed

	for _, seg := range segments[1:] {
		// there's a gap between segments; stop serving the recording.
		if seg.start.Before(start.Add(-concatenationTolerance)) || seg.start.After(start.Add(concatenationTolerance)) {
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

		start = seg.start.Add(elapsed)
		duration -= elapsed
		overallElapsed += elapsed
	}
}
