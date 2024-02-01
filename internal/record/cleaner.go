package record

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
)

var timeNow = time.Now

// CleanerEntry is a cleaner entry.
type CleanerEntry struct {
	Path        string
	Format      conf.RecordFormat
	DeleteAfter time.Duration
}

// Cleaner removes expired recording segments from disk.
type Cleaner struct {
	Entries []CleanerEntry
	Parent  logger.Writer

	ctx       context.Context
	ctxCancel func()

	done chan struct{}
}

// Initialize initializes a Cleaner.
func (c *Cleaner) Initialize() {
	c.ctx, c.ctxCancel = context.WithCancel(context.Background())
	c.done = make(chan struct{})

	go c.run()
}

// Close closes the Cleaner.
func (c *Cleaner) Close() {
	c.ctxCancel()
	<-c.done
}

// Log implements logger.Writer.
func (c *Cleaner) Log(level logger.Level, format string, args ...interface{}) {
	c.Parent.Log(level, "[record cleaner]"+format, args...)
}

func (c *Cleaner) run() {
	defer close(c.done)

	interval := 30 * 60 * time.Second
	for _, e := range c.Entries {
		if interval > (e.DeleteAfter / 2) {
			interval = e.DeleteAfter / 2
		}
	}

	c.doRun() //nolint:errcheck

	for {
		select {
		case <-time.After(interval):
			c.doRun()

		case <-c.ctx.Done():
			return
		}
	}
}

func (c *Cleaner) doRun() {
	for _, e := range c.Entries {
		c.doRunEntry(&e) //nolint:errcheck
	}
}

func (c *Cleaner) doRunEntry(e *CleanerEntry) error {
	entryPath := PathAddExtension(e.Path, e.Format)

	// we have to convert to absolute paths
	// otherwise, entryPath and fpath inside Walk() won't have common elements
	entryPath, _ = filepath.Abs(entryPath)

	commonPath := CommonPath(entryPath)
	now := timeNow()

	filepath.Walk(commonPath, func(fpath string, info fs.FileInfo, err error) error { //nolint:errcheck
		if err != nil {
			return err
		}

		if !info.IsDir() {
			var pa Path
			ok := pa.Decode(entryPath, fpath)
			if ok {
				if now.Sub(pa.Start) > e.DeleteAfter {
					c.Log(logger.Debug, "removing %s", fpath)
					os.Remove(fpath)
				}
			}
		}

		return nil
	})

	filepath.Walk(commonPath, func(fpath string, info fs.FileInfo, err error) error { //nolint:errcheck
		if err != nil {
			return err
		}

		if info.IsDir() {
			os.Remove(fpath)
		}

		return nil
	})

	return nil
}
