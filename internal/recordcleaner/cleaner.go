// Package recordcleaner contains the recording cleaner.
package recordcleaner

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/recordstore"
)

var timeNow = time.Now

type segmentEntry struct {
	fpath    string
	start    time.Time
	size     int64
	pathName string
}

// Cleaner removes expired recording segments from disk.
type Cleaner struct {
	PathConfs           map[string]*conf.Path
	RecordDeleteMaxSize conf.StringSize
	Parent              logger.Writer

	ctx       context.Context
	ctxCancel func()

	chReloadConf chan reloadConf
	done         chan struct{}
}

type reloadConf struct {
	pathConfs           map[string]*conf.Path
	recordDeleteMaxSize conf.StringSize
}

// Initialize initializes a Cleaner.
func (c *Cleaner) Initialize() {
	c.ctx, c.ctxCancel = context.WithCancel(context.Background())
	c.chReloadConf = make(chan reloadConf)
	c.done = make(chan struct{})

	go c.run()
}

// Close closes a Cleaner.
func (c *Cleaner) Close() {
	c.ctxCancel()
	<-c.done
}

// Log implements logger.Writer.
func (c *Cleaner) Log(level logger.Level, format string, args ...any) {
	c.Parent.Log(level, "[record cleaner]"+format, args...)
}

// ReloadPathConfs is called by core.Core.
func (c *Cleaner) ReloadPathConfs(pathConfs map[string]*conf.Path, recordDeleteMaxSize conf.StringSize) {
	select {
	case c.chReloadConf <- reloadConf{
		pathConfs:           pathConfs,
		recordDeleteMaxSize: recordDeleteMaxSize,
	}:
	case <-c.ctx.Done():
	}
}

func (c *Cleaner) run() {
	defer close(c.done)

	c.doRun() //nolint:errcheck

	for {
		select {
		case <-time.After(c.cleanInterval()):
			c.doRun()

		case cnf := <-c.chReloadConf:
			c.PathConfs = cnf.pathConfs
			c.RecordDeleteMaxSize = cnf.recordDeleteMaxSize

		case <-c.ctx.Done():
			return
		}
	}
}

func (c *Cleaner) cleanInterval() time.Duration {
	interval := 30 * 60 * time.Second

	if c.RecordDeleteMaxSize != 0 {
		interval = 60 * time.Second
	}

	for _, e := range c.PathConfs {
		if e.RecordDeleteAfter != 0 &&
			interval > (time.Duration(e.RecordDeleteAfter)/2) {
			interval = time.Duration(e.RecordDeleteAfter) / 2
		}
	}

	return interval
}

func (c *Cleaner) doRun() {
	now := timeNow()

	pathNames := recordstore.FindAllPathsWithSegments(c.PathConfs)

	for _, pathName := range pathNames {
		c.processPath(now, pathName) //nolint:errcheck
	}

	c.deleteOverflowSegments()
}

func (c *Cleaner) processPath(now time.Time, pathName string) error {
	pathConf, _, err := conf.FindPathConf(c.PathConfs, pathName)
	if err != nil {
		return err
	}

	if pathConf.RecordDeleteAfter == 0 {
		return nil
	}

	err = c.deleteExpiredSegments(now, pathName, pathConf)
	if err != nil {
		return err
	}

	c.deleteEmptyDirs(pathConf)

	return nil
}

func (c *Cleaner) deleteExpiredSegments(now time.Time, pathName string, pathConf *conf.Path) error {
	end := now.Add(-time.Duration(pathConf.RecordDeleteAfter))
	segments, err := recordstore.FindSegments(pathConf, pathName, nil, &end)
	if err != nil {
		return err
	}

	for _, seg := range segments {
		c.Log(logger.Debug, "removing %s", seg.Fpath)
		os.Remove(seg.Fpath)
	}

	return nil
}

func (c *Cleaner) deleteOverflowSegments() {
	if c.RecordDeleteMaxSize == 0 {
		return
	}

	entries, err := c.gatherSegmentEntries()
	if err != nil {
		return
	}

	var totalSize uint64
	for _, e := range entries {
		totalSize += uint64(e.size)
	}

	limit := uint64(c.RecordDeleteMaxSize)
	if totalSize <= limit {
		return
	}

	newestByPath := make(map[string]time.Time)
	for _, e := range entries {
		if t, ok := newestByPath[e.pathName]; !ok || e.start.After(t) {
			newestByPath[e.pathName] = e.start
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].start.Before(entries[j].start)
	})

	touchedPathConfs := make(map[*conf.Path]struct{})

	for _, e := range entries {
		if totalSize <= limit {
			break
		}

		if e.start.Equal(newestByPath[e.pathName]) {
			continue
		}

		c.Log(logger.Debug, "removing %s (storage limit)", e.fpath)
		err = os.Remove(e.fpath)
		if err != nil {
			continue
		}

		totalSize -= uint64(e.size)

		pathConf, _, findErr := conf.FindPathConf(c.PathConfs, e.pathName)
		if findErr == nil {
			touchedPathConfs[pathConf] = struct{}{}
		}
	}

	for pathConf := range touchedPathConfs {
		c.deleteEmptyDirs(pathConf)
	}
}

func (c *Cleaner) gatherSegmentEntries() ([]segmentEntry, error) {
	pathNames := recordstore.FindAllPathsWithSegments(c.PathConfs)
	var entries []segmentEntry

	for _, pathName := range pathNames {
		pathConf, _, err := conf.FindPathConf(c.PathConfs, pathName)
		if err != nil {
			continue
		}

		segments, err := recordstore.FindSegments(pathConf, pathName, nil, nil)
		if err != nil {
			if errors.Is(err, recordstore.ErrNoSegmentsFound) {
				continue
			}
			return nil, err
		}

		for _, seg := range segments {
			fi, err2 := os.Stat(seg.Fpath)
			if err2 != nil {
				continue
			}

			entries = append(entries, segmentEntry{
				fpath:    seg.Fpath,
				start:    seg.Start,
				size:     fi.Size(),
				pathName: pathName,
			})
		}
	}

	return entries, nil
}

func (c *Cleaner) deleteEmptyDirs(pathConf *conf.Path) {
	recordPath := strings.ReplaceAll(pathConf.RecordPath, "%path", pathConf.Name)
	commonPath := recordstore.CommonPath(recordPath)

	filepath.WalkDir(commonPath, func(fpath string, info fs.DirEntry, err error) error { //nolint:errcheck
		if err != nil {
			return err
		}

		if info.IsDir() {
			os.Remove(fpath)
		}

		return nil
	})
}
