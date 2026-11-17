// Package watchusage counts watched milliseconds of served live video segments.
package watchusage

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/models"
)

type key struct {
	mediaID   uuid.UUID
	rendition string
}

// Counter aggregates watched ms and flushes them to the repository.
type Counter struct {
	repo models.LiveWatchUsageRepository

	mutex   sync.Mutex
	pending map[key]int64

	flushMutex sync.Mutex
	done       chan struct{}
	wg         sync.WaitGroup
	closeOnce  sync.Once
}

// New creates a Counter that flushes every flushEvery.
func New(repo models.LiveWatchUsageRepository, flushEvery time.Duration) *Counter {
	c := &Counter{
		repo:    repo,
		pending: make(map[key]int64),
		done:    make(chan struct{}),
	}
	if flushEvery > 0 {
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			t := time.NewTicker(flushEvery)
			defer t.Stop()
			for {
				select {
				case <-t.C:
					c.flush()
				case <-c.done:
					return
				}
			}
		}()
	}
	return c
}

// Billable reports whether a rendition is billed.
func Billable(rendition string) bool {
	return rendition == "720" || rendition == "1080"
}

// RenditionFromPath parses "<uuid>/<rendition>" or "<uuid>/video/<rendition>".
func RenditionFromPath(pathName string) (uuid.UUID, string, bool) {
	parts := strings.Split(strings.Trim(pathName, "/"), "/")
	if len(parts) < 2 {
		return uuid.Nil, "", false
	}
	id, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, "", false
	}
	r := parts[len(parts)-1]
	if r == "" {
		return uuid.Nil, "", false
	}
	for _, ch := range r {
		if ch < '0' || ch > '9' {
			return uuid.Nil, "", false
		}
	}
	return id, r, true
}

// Add records ms watched; non-billable renditions and ms<=0 are ignored.
func (c *Counter) Add(mediaID uuid.UUID, rendition string, ms int64) {
	if c == nil || ms <= 0 || !Billable(rendition) {
		return
	}
	c.mutex.Lock()
	c.pending[key{mediaID, rendition}] += ms
	c.mutex.Unlock()
}

func (c *Counter) flush() {
	if c.repo == nil {
		return
	}
	c.flushMutex.Lock()
	defer c.flushMutex.Unlock()

	c.mutex.Lock()
	batch := c.pending
	c.pending = make(map[key]int64)
	c.mutex.Unlock()

	for k, ms := range batch {
		if err := c.repo.AddWatchedMS(k.mediaID, k.rendition, ms); err != nil {
			c.mutex.Lock()
			c.pending[k] += ms
			c.mutex.Unlock()
		}
	}
}

// Close stops the ticker and flushes once.
func (c *Counter) Close(_ context.Context) {
	if c == nil {
		return
	}
	c.closeOnce.Do(func() { close(c.done) })
	c.wg.Wait()
	c.flush()
}
