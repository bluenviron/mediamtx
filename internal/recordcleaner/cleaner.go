// Package recordcleaner contains the recording cleaner.
package recordcleaner

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/recordstore"
)

var timeNow = time.Now

// recordPathOf returns the absolute record path of a path configuration, with extension.
func recordPathOf(pathConf *conf.Path) string {
	recordPath := recordstore.PathAddExtension(pathConf.RecordPath, pathConf.RecordFormat)

	// we have to convert to absolute paths
	// otherwise, recordPath and fpath inside Walk() won't have common elements
	recordPath, _ = filepath.Abs(recordPath)

	return recordPath
}

// Cleaner removes expired recording segments from disk.
type Cleaner struct {
	PathConfs map[string]*conf.Path
	Parent    logger.Writer

	ctx       context.Context
	ctxCancel func()

	// the most recent configuration passed to ReloadPathConfs,
	// not yet picked up by the cleaner routine.
	pendingMutex     sync.Mutex
	pendingPathConfs map[string]*conf.Path

	chReloadConf chan struct{}
	done         chan struct{}
}

// Initialize initializes a Cleaner.
func (c *Cleaner) Initialize() {
	c.ctx, c.ctxCancel = context.WithCancel(context.Background())
	c.chReloadConf = make(chan struct{}, 1)
	c.done = make(chan struct{})

	go c.run()
}

// Close closes the Cleaner.
func (c *Cleaner) Close() {
	c.ctxCancel()
	<-c.done
}

// Log implements logger.Writer.
func (c *Cleaner) Log(level logger.Level, format string, args ...any) {
	c.Parent.Log(level, "[record cleaner]"+format, args...)
}

// ReloadPathConfs is called by core.Core.
// It never waits for a cleaning pass, which can last long on large directories.
// The new configuration is used starting from the next pass.
func (c *Cleaner) ReloadPathConfs(pathConfs map[string]*conf.Path) {
	c.pendingMutex.Lock()
	c.pendingPathConfs = pathConfs
	c.pendingMutex.Unlock()

	select {
	case c.chReloadConf <- struct{}{}:
	default:
	}
}

func (c *Cleaner) applyPendingPathConfs() {
	c.pendingMutex.Lock()
	defer c.pendingMutex.Unlock()

	if c.pendingPathConfs != nil {
		c.PathConfs = c.pendingPathConfs
		c.pendingPathConfs = nil
	}
}

func (c *Cleaner) run() {
	defer close(c.done)

	c.doRun()

	timer := time.NewTimer(c.cleanInterval())
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			c.doRun()
			timer.Reset(c.cleanInterval())

		case <-c.chReloadConf:
			c.applyPendingPathConfs()

		case <-c.ctx.Done():
			return
		}
	}
}

func (c *Cleaner) cleanInterval() time.Duration {
	interval := 30 * 60 * time.Second

	for _, e := range c.PathConfs {
		if e.RecordDeleteAfter != 0 &&
			interval > (time.Duration(e.RecordDeleteAfter)/2) {
			interval = time.Duration(e.RecordDeleteAfter) / 2
		}
	}

	return interval
}

// pass is a cleaning pass, performed with the configuration available when it starts.
type pass struct {
	c         *Cleaner
	now       time.Time
	pathConfs map[string]*conf.Path

	// path configuration of each path name, nil when there's none.
	resolved map[string]*conf.Path

	// absolute record path of each path configuration.
	recordPaths map[*conf.Path]string

	// directories that contain segments of paths that have been cleaned.
	cleanedDirs map[string]struct{}
}

func (c *Cleaner) doRun() {
	c.applyPendingPathConfs()

	p := &pass{
		c:           c,
		now:         timeNow(),
		pathConfs:   c.PathConfs,
		resolved:    make(map[string]*conf.Path),
		recordPaths: make(map[*conf.Path]string),
		cleanedDirs: make(map[string]struct{}),
	}

	// segments of several paths often share the same directory,
	// for instance when recordPath is "./recordings/%path_%Y-%m-%d_%H-%M-%S-%f".
	// Walk each directory once, instead of once per path.
	formatsByCommonPath := make(map[string]map[string]struct{})

	for _, pathConf := range c.PathConfs {
		recordPath := recordPathOf(pathConf)
		p.recordPaths[pathConf] = recordPath
		commonPath := recordstore.CommonPath(recordPath)

		if formatsByCommonPath[commonPath] == nil {
			formatsByCommonPath[commonPath] = make(map[string]struct{})
		}
		formatsByCommonPath[commonPath][recordPath] = struct{}{}
	}

	commonPaths := make([]string, 0, len(formatsByCommonPath))
	for commonPath := range formatsByCommonPath {
		commonPaths = append(commonPaths, commonPath)
	}
	sort.Strings(commonPaths)

	for _, commonPath := range commonPaths {
		err := p.cleanCommonPath(commonPath, formatsByCommonPath[commonPath])
		if err != nil && c.ctx.Err() != nil {
			return
		}
	}
}

type format struct {
	recordPath string
	decoder    *recordstore.PathDecoder
}

func (p *pass) cleanCommonPath(commonPath string, recordPaths map[string]struct{}) error {
	formats := make([]format, 0, len(recordPaths))
	for recordPath := range recordPaths {
		formats = append(formats, format{
			recordPath: recordPath,
			decoder:    recordstore.NewPathDecoder(recordPath),
		})
	}
	sort.Slice(formats, func(i, j int) bool {
		return formats[i].recordPath < formats[j].recordPath
	})

	var dirs []string

	err := filepath.WalkDir(commonPath, func(fpath string, info fs.DirEntry, err error) error {
		if p.c.ctx.Err() != nil {
			return p.c.ctx.Err()
		}

		if err != nil {
			// skip entries that cannot be read, like directories removed during the walk.
			if fpath == commonPath {
				return err
			}
			return nil
		}

		if info.IsDir() {
			dirs = append(dirs, fpath)
			return nil
		}

		p.processFile(fpath, formats)
		return nil
	})
	if err != nil {
		return err
	}

	p.deleteEmptyDirs(dirs)

	return nil
}

func (p *pass) resolvePathConf(pathName string) *conf.Path {
	if pathConf, ok := p.resolved[pathName]; ok {
		return pathConf
	}

	pathConf, _, err := conf.FindPathConf(p.pathConfs, pathName)
	if err != nil {
		pathConf = nil
	}

	p.resolved[pathName] = pathConf
	return pathConf
}

func (p *pass) processFile(fpath string, formats []format) {
	for _, f := range formats {
		var pa recordstore.Path
		if !f.decoder.Decode(&pa, fpath) {
			continue
		}

		if err := conf.IsValidPathName(pa.Path); err != nil {
			continue
		}

		// the segment belongs to the path configuration of its path name,
		// and only if that configuration produces this format.
		pathConf := p.resolvePathConf(pa.Path)
		if pathConf == nil || p.recordPaths[pathConf] != f.recordPath {
			continue
		}

		if pathConf.RecordDeleteAfter == 0 {
			return
		}

		segmentCommonPath := recordstore.CommonPath(
			strings.ReplaceAll(f.recordPath, "%path", pa.Path))
		p.cleanedDirs[segmentCommonPath] = struct{}{}

		end := p.now.Add(-time.Duration(pathConf.RecordDeleteAfter))
		if !end.Before(pa.Start) {
			p.c.Log(logger.Debug, "removing %s", fpath)
			os.Remove(fpath)
		}

		return
	}
}

// deleteEmptyDirs removes empty directories inside the common path of paths
// that have been cleaned.
func (p *pass) deleteEmptyDirs(dirs []string) {
	// walk directories in reverse order, in order to remove children before parents.
	for _, dir := range slices.Backward(dirs) {
		if p.c.ctx.Err() != nil {
			return
		}

		if p.isInsideCleanedDir(dir) {
			os.Remove(dir)
		}
	}
}

func (p *pass) isInsideCleanedDir(dir string) bool {
	for cleanedDir := range p.cleanedDirs {
		if dir == cleanedDir || strings.HasPrefix(dir, cleanedDir+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
