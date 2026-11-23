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
	segment   string
}

// Counter collects the segments served since the last flush and hands them to
// the repository. How a segment is billed depends on billOnce:
//   - true: once per (stream, rendition, segment), however often it is
//     requested; the repository remembers what it has counted.
//   - false: once per request, each served segment adding its full duration.
type Counter struct {
	repo     models.LiveWatchUsageRepository
	billOnce bool

	mutex   sync.Mutex
	pending map[key]int64

	flushMutex sync.Mutex
	done       chan struct{}
	wg         sync.WaitGroup
	closeOnce  sync.Once
}

// Option adjusts a Counter.
type Option func(*Counter)

// WithBillOnce sets whether a segment is billed once (true, the default) or on
// every request (false).
func WithBillOnce(once bool) Option {
	return func(c *Counter) { c.billOnce = once }
}

// New creates a Counter that flushes every flushEvery.
func New(repo models.LiveWatchUsageRepository, flushEvery time.Duration, opts ...Option) *Counter {
	c := &Counter{
		repo:     repo,
		billOnce: true,
		pending:  make(map[key]int64),
		done:     make(chan struct{}),
	}
	for _, opt := range opts {
		opt(c)
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

// Billable reports whether a rendition is billed: the renditions the live
// transcoder produces. The API prices each of them (480 at the 720 price unless
// configured otherwise) and skips any other name.
func Billable(rendition string) bool {
	return rendition == "480" || rendition == "720" || rendition == "1080"
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

// Add records that segment of rendition was served. Non-billable renditions
// and ms<=0 are ignored.
//
// When billing once, repeats of the same segment within a flush window collapse
// to one and an empty segment name is ignored. When billing per request, every
// call adds ms and the segment name does not matter.
func (c *Counter) Add(mediaID uuid.UUID, rendition, segment string, ms int64) {
	if c == nil || ms <= 0 || !Billable(rendition) {
		return
	}
	if !c.billOnce {
		c.mutex.Lock()
		c.pending[key{mediaID: mediaID, rendition: rendition}] += ms
		c.mutex.Unlock()

		return
	}
	if segment == "" {
		return
	}
	c.mutex.Lock()
	k := key{mediaID, rendition, segment}
	if _, seen := c.pending[k]; !seen {
		c.pending[k] = ms
	}
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
		if !c.billOnce {
			if err := c.repo.AddWatchedMS(k.mediaID, k.rendition, ms); err != nil {
				c.mutex.Lock()
				c.pending[k] += ms
				c.mutex.Unlock()
			}

			continue
		}

		if err := c.repo.AddSegmentWatch(k.mediaID, k.rendition, k.segment, ms); err != nil {
			// Safe to retry: the repository counts a segment once.
			c.mutex.Lock()
			if _, seen := c.pending[k]; !seen {
				c.pending[k] = ms
			}
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
