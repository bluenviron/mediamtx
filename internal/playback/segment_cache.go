package playback

import (
	"container/list"
	"os"
	"sync"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
)

// maximum number of entries in the segment cache.
// each entry holds the segment path, its parsed init (tracks, codec parameters,
// user data) and the map and list overhead, which amounts to about 1 KB,
// therefore the cache uses about 10 MB at most.
const segmentCacheMaxEntries = 10000

type segmentCacheEntry struct {
	fpath    string
	size     int64
	modTime  time.Time
	init     *fmp4.Init
	duration time.Duration
}

// segmentCache is an in-memory, LRU-bounded cache of segment headers,
// that allows to avoid opening and parsing every segment on each /list call.
//
// Only headers of closed segments are cached (i.e. segments whose duration
// has been written into mvhd by the recorder), since mediamtx never modifies
// a segment after it has been closed.
//
// Entries are validated against the size and modification time of the file,
// which are gathered before the header is read: a concurrent modification
// can cause an additional parsing at most, never a stale hit.
//
// Cached inits are shared between requests and must be treated as read-only.
type segmentCache struct {
	maxEntries int

	mutex   sync.Mutex
	entries map[string]*list.Element
	lru     *list.List // front = most recently used
}

func (c *segmentCache) initialize() {
	if c.maxEntries == 0 {
		c.maxEntries = segmentCacheMaxEntries
	}
	c.entries = make(map[string]*list.Element)
	c.lru = list.New()
}

// get returns the cached header of a segment,
// or false if the segment is not cached or the cached entry is outdated.
func (c *segmentCache) get(fpath string, fi os.FileInfo) (*fmp4.Init, time.Duration, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	el, ok := c.entries[fpath]
	if !ok {
		return nil, 0, false
	}

	e := el.Value.(*segmentCacheEntry)

	if e.size != fi.Size() || !e.modTime.Equal(fi.ModTime()) {
		c.lru.Remove(el)
		delete(c.entries, fpath)
		return nil, 0, false
	}

	c.lru.MoveToFront(el)

	return e.init, e.duration, true
}

// set stores the header of a segment.
func (c *segmentCache) set(fpath string, fi os.FileInfo, init *fmp4.Init, duration time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	e := &segmentCacheEntry{
		fpath:    fpath,
		size:     fi.Size(),
		modTime:  fi.ModTime(),
		init:     init,
		duration: duration,
	}

	if el, ok := c.entries[fpath]; ok {
		el.Value = e
		c.lru.MoveToFront(el)
		return
	}

	c.entries[fpath] = c.lru.PushFront(e)

	for c.lru.Len() > c.maxEntries {
		last := c.lru.Back()
		c.lru.Remove(last)
		delete(c.entries, last.Value.(*segmentCacheEntry).fpath)
	}
}

// remove removes the header of a segment, if present.
func (c *segmentCache) remove(fpath string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if el, ok := c.entries[fpath]; ok {
		c.lru.Remove(el)
		delete(c.entries, fpath)
	}
}

// count returns the number of cached headers.
func (c *segmentCache) count() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.lru.Len()
}
