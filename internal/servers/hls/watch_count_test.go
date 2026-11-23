package hls

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type watchAdd struct {
	id uuid.UUID
	r  string
	ms int64
}

type fakeWatchCounter struct {
	mu   sync.Mutex
	adds []watchAdd
}

func (f *fakeWatchCounter) Add(id uuid.UUID, r string, ms int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.adds = append(f.adds, watchAdd{id, r, ms})
}

type fakeSegRepo struct {
	models.LiveHLSSegmentRepository
	ms int64
}

func (f fakeSegRepo) GetByStreamAndSegment(_, _ string) (*models.LiveHLSSegment, error) {
	return &models.LiveHLSSegment{DurationMS: f.ms}, nil
}

func newWatchMI(path string, repoMS int64, c *fakeWatchCounter) *muxerInstance {
	return &muxerInstance{
		pathName:        path,
		segmentDuration: conf.Duration(6 * time.Second),
		watchCounter:    c,
		segmentRepo:     fakeSegRepo{ms: repoMS},
	}
}

func TestHandleRequestCountsBilledVideoSegmentWatchCount(t *testing.T) {
	id := uuid.New()
	c := &fakeWatchCounter{}
	mi := newWatchMI(id.String()+"/720", 2000, c)
	mi.countWatch("video1_seg0.mp4")
	require.Equal(t, []watchAdd{{id, "720", 2000}}, c.adds)

	c2 := &fakeWatchCounter{}
	mi = newWatchMI(id.String()+"/1080", 0, c2)
	mi.countWatch("seg1.ts")
	require.Equal(t, []watchAdd{{id, "1080", 6000}}, c2.adds)
}

func TestHandleRequestIgnoresAudioInitAndPartWatchCount(t *testing.T) {
	id := uuid.New()
	c := &fakeWatchCounter{}
	for _, rendition := range []string{"480", "720", "1080"} {
		mi := newWatchMI(id.String()+"/"+rendition, 2000, c)
		for _, f := range []string{"x_audio_1.mp4", "audio2_seg0.mp4", "video_audio2_seg0.mp4", "x_init.mp4", "x_part1.mp4", "main_stream.m3u8"} {
			mi.countWatch(f)
		}
	}
	require.Empty(t, c.adds)
}

func TestHandleRequestCounts480WatchCount(t *testing.T) {
	id := uuid.New()
	c := &fakeWatchCounter{}
	mi := newWatchMI(id.String()+"/480", 2000, c)
	mi.countWatch("video1_seg0.mp4")
	require.Equal(t, []watchAdd{{id, "480", 2000}}, c.adds)

	// A rendition the transcoder does not produce is still not counted.
	c2 := &fakeWatchCounter{}
	mi = newWatchMI(id.String()+"/360", 2000, c2)
	mi.countWatch("video1_seg0.mp4")
	require.Empty(t, c2.adds)
}

func TestHandleRequestCountsLowLatencyVideoSegmentWatchCount(t *testing.T) {
	id := uuid.New()
	c := &fakeWatchCounter{}
	mi := newWatchMI(id.String()+"/720", 2000, c)
	mi.variant = conf.HLSVariant(gohlslib.MuxerVariantLowLatency)
	mi.bytesSent = &atomic.Uint64{}
	mi.serveHLS = func(http.ResponseWriter, *http.Request) {}
	rec := httptest.NewRecorder()
	gc, _ := gin.CreateTestContext(rec)
	for _, f := range []string{"video1_seg0_part1.mp4", "video1_seg0.mp4"} {
		gc.Request = httptest.NewRequest("GET", "/"+id.String()+"/720/"+f, nil)
		mi.handleRequest(gc, false)
	}
	require.Equal(t, []watchAdd{{id, "720", 2000}}, c.adds)
}
