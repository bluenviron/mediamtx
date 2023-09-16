package record

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// Cleaner removes expired recordings from disk.
type Cleaner struct {
	ctx         context.Context
	ctxCancel   func()
	path        string
	deleteAfter time.Duration
	parent      logger.Writer

	done chan struct{}
}

// NewCleaner allocates a Cleaner.
func NewCleaner(
	recordPath string,
	deleteAfter time.Duration,
	parent logger.Writer,
) *Cleaner {
	recordPath, _ = filepath.Abs(recordPath)
	recordPath += ".mp4"

	ctx, ctxCancel := context.WithCancel(context.Background())

	c := &Cleaner{
		ctx:         ctx,
		ctxCancel:   ctxCancel,
		path:        recordPath,
		deleteAfter: deleteAfter,
		parent:      parent,
		done:        make(chan struct{}),
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
	if interval > (c.deleteAfter / 2) {
		interval = c.deleteAfter / 2
	}

	c.doRun() //nolint:errcheck

	for {
		select {
		case <-time.After(interval):
			c.doRun() //nolint:errcheck

		case <-c.ctx.Done():
			return
		}
	}
}

func (c *Cleaner) doRun() error {
	commonPath := commonPath(c.path)
	now := timeNow()

	filepath.Walk(commonPath, func(path string, info fs.FileInfo, err error) error { //nolint:errcheck
		if err != nil {
			return err
		}

		if !info.IsDir() {
			params := decodeRecordPath(c.path, path)
			if params != nil {
				if now.Sub(params.time) > c.deleteAfter {
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
