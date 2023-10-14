package record

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
)

func commonPath(v string) string {
	common := ""
	remaining := v

	for {
		i := strings.IndexAny(remaining, "\\/")
		if i < 0 {
			break
		}

		var part string
		part, remaining = remaining[:i+1], remaining[i+1:]

		if strings.Contains(part, "%") {
			break
		}

		common += part
	}

	if len(common) > 0 {
		common = common[:len(common)-1]
	}

	return common
}

// CleanerEntry is a cleaner entry.
type CleanerEntry struct {
	RecordPath        string
	RecordFormat      conf.RecordFormat
	RecordDeleteAfter time.Duration
}

// Cleaner removes expired recording segments from disk.
type Cleaner struct {
	ctx       context.Context
	ctxCancel func()
	entries   []CleanerEntry
	parent    logger.Writer

	done chan struct{}
}

// NewCleaner allocates a Cleaner.
func NewCleaner(
	entries []CleanerEntry,
	parent logger.Writer,
) *Cleaner {
	ctx, ctxCancel := context.WithCancel(context.Background())

	c := &Cleaner{
		ctx:       ctx,
		ctxCancel: ctxCancel,
		entries:   entries,
		parent:    parent,
		done:      make(chan struct{}),
	}

	go c.run()

	return c
}

// Close closes the Cleaner.
func (c *Cleaner) Close() {
	c.ctxCancel()
	<-c.done
}

// Log is the main logging function.
func (c *Cleaner) Log(level logger.Level, format string, args ...interface{}) {
	c.parent.Log(level, "[record cleaner]"+format, args...)
}

func (c *Cleaner) run() {
	defer close(c.done)

	interval := 30 * 60 * time.Second
	for _, e := range c.entries {
		if interval > (e.RecordDeleteAfter / 2) {
			interval = e.RecordDeleteAfter / 2
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
	for _, e := range c.entries {
		c.doRunEntry(&e) //nolint:errcheck
	}
}

func (c *Cleaner) doRunEntry(e *CleanerEntry) error {
	recordPath := e.RecordPath

	switch e.RecordFormat {
	case conf.RecordFormatMPEGTS:
		recordPath += ".ts"

	default:
		recordPath += ".mp4"
	}

	commonPath := commonPath(recordPath)
	now := timeNow()

	filepath.Walk(commonPath, func(path string, info fs.FileInfo, err error) error { //nolint:errcheck
		if err != nil {
			return err
		}

		if !info.IsDir() {
			params := decodeRecordPath(recordPath, path)
			if params != nil {
				if now.Sub(params.time) > e.RecordDeleteAfter {
					c.Log(logger.Debug, "removing %s", path)
					os.Remove(path)
				}
			}
		}

		return nil
	})

	filepath.Walk(commonPath, func(path string, info fs.FileInfo, err error) error { //nolint:errcheck
		if err != nil {
			return err
		}

		if info.IsDir() {
			os.Remove(path)
		}

		return nil
	})

	return nil
}
