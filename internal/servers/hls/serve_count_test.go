package hls

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/dvr"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/models"
)

// serveRecord is one call to the counter under test.
type serveRecord struct {
	stream, segment, outcome string
}

type fakeServeCounter struct {
	mu    sync.Mutex
	calls []serveRecord
}

func (f *fakeServeCounter) RecordServe(stream, segment, outcome string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, serveRecord{stream, segment, outcome})
}

func (f *fakeServeCounter) got() []serveRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]serveRecord(nil), f.calls...)
}

// quietParent is the instance's parent; it only has to swallow log lines.
type quietParent struct{}

func (quietParent) Log(logger.Level, string, ...any)    {}
func (quietParent) closeInstance(*muxerInstance, error) {}

// fakeUploader is a segmentUploader whose storage answers are set per test.
type fakeUploader struct {
	uploaded   bool
	proxyOK    bool
	presignURL string
	presignErr error

	proxied   atomic.Int32
	presigned atomic.Int32
}

func (f *fakeUploader) BuildRemoteKey(stream, file string) string {
	return "live/" + stream + "/" + file
}
func (f *fakeUploader) IsUploaded(string) bool { return f.uploaded }
func (f *fakeUploader) FlushAndClose()         {}

func (f *fakeUploader) ProxyObject(_ context.Context, w http.ResponseWriter, _ string) bool {
	f.proxied.Add(1)
	if !f.proxyOK {
		return false
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("segment-bytes"))
	return true
}

func (f *fakeUploader) Presign(string) (string, error) {
	f.presigned.Add(1)
	return f.presignURL, f.presignErr
}

// newServeMI builds an ABR-style (non low-latency) instance. directory is the
// local muxer directory the segment is looked up in; an empty one makes every
// segment count as locally available.
func newServeMI(t *testing.T, directory string, up segmentUploader, c ServeCounter) (*muxerInstance, *int) {
	t.Helper()

	muxerServed := new(int)
	mi := &muxerInstance{
		pathName:     "stream-1/720",
		directory:    directory,
		bytesSent:    &atomic.Uint64{},
		uploader:     up,
		serveCounter: c,
		parent:       quietParent{},
		serveHLS: func(w http.ResponseWriter, _ *http.Request) {
			*muxerServed++
			w.WriteHeader(http.StatusOK)
		},
	}

	return mi, muxerServed
}

// servedResponse is what the client sees. The status is read from gin's writer:
// gin defers ctx.Status() until the first write, so a handler that only sets
// 404 leaves the recorder's own Code at its default of 200.
type servedResponse struct {
	*httptest.ResponseRecorder
	status int
}

func serve(mi *muxerInstance, file string) servedResponse {
	rec := httptest.NewRecorder()
	gc, _ := gin.CreateTestContext(rec)
	gc.Request = httptest.NewRequest(http.MethodGet, "/stream-1/720/"+file, nil)
	mi.handleRequest(gc, false)

	return servedResponse{ResponseRecorder: rec, status: gc.Writer.Status()}
}

// ----- the branches of handleRequest and what each counts ---------------------

func TestHandleRequestCountsLocalWhenSegmentIsOnDisk(t *testing.T) {
	// No directory configured: every segment is served from the muxer itself.
	c := &fakeServeCounter{}
	mi, muxerServed := newServeMI(t, "", &fakeUploader{}, c)

	serve(mi, "seg0.mp4")

	require.Equal(t, 1, *muxerServed)
	require.Equal(t, []serveRecord{{"stream-1/720", "seg0.mp4", dvr.ServeOutcomeLocal}}, c.got())
}

func TestHandleRequestCountsProxyWhenLocalSegmentIsGone(t *testing.T) {
	// The directory exists but holds no segment: it left the local window.
	c := &fakeServeCounter{}
	up := &fakeUploader{uploaded: true, proxyOK: true}
	mi, muxerServed := newServeMI(t, t.TempDir(), up, c)

	rec := serve(mi, "seg0.mp4")

	require.Equal(t, http.StatusOK, rec.status)
	require.Equal(t, "segment-bytes", rec.Body.String())
	require.Zero(t, *muxerServed, "served from the provider, not the muxer")
	require.EqualValues(t, 1, up.proxied.Load())
	require.Zero(t, up.presigned.Load(), "a proxied segment is not also redirected")
	require.Equal(t, []serveRecord{{"stream-1/720", "seg0.mp4", dvr.ServeOutcomeProxy}}, c.got())
}

func TestHandleRequestCountsRedirectWhenProxyIsUnavailable(t *testing.T) {
	c := &fakeServeCounter{}
	up := &fakeUploader{uploaded: true, proxyOK: false, presignURL: "https://cdn.example/seg0.mp4"}
	mi, _ := newServeMI(t, t.TempDir(), up, c)

	rec := serve(mi, "seg0.mp4")

	require.Equal(t, http.StatusFound, rec.status)
	require.Equal(t, "https://cdn.example/seg0.mp4", rec.Header().Get("Location"))
	require.EqualValues(t, 1, up.proxied.Load(), "proxy is tried first")
	require.EqualValues(t, 1, up.presigned.Load())
	require.Equal(t, []serveRecord{{"stream-1/720", "seg0.mp4", dvr.ServeOutcomeRedirect}}, c.got())
}

func TestHandleRequestCountsMissWhenNothingCanServeTheSegment(t *testing.T) {
	tests := []struct {
		name   string
		up     segmentUploader
		status int
	}{
		{"no uploader", nil, http.StatusNotFound},
		{"never uploaded", &fakeUploader{uploaded: false}, http.StatusNotFound},
		{"proxy and presign both fail", &fakeUploader{uploaded: true, presignErr: errors.New("no link")}, http.StatusBadGateway},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &fakeServeCounter{}
			mi, _ := newServeMI(t, t.TempDir(), tt.up, c)

			rec := serve(mi, "seg0.mp4")

			require.Equal(t, tt.status, rec.status)
			require.Equal(t, []serveRecord{{"stream-1/720", "seg0.mp4", dvr.ServeOutcomeMiss}}, c.got())
		})
	}
}

func TestHandleRequestCountsServesOnlyForMediaSegments(t *testing.T) {
	c := &fakeServeCounter{}
	mi, _ := newServeMI(t, "", &fakeUploader{}, c)

	for _, f := range []string{"main_stream.m3u8", "video_init.mp4", "video1_seg0_part1.mp4"} {
		serve(mi, f)
	}
	require.Empty(t, c.got(), "playlists, init segments and parts have no row to count against")

	serve(mi, "video1_seg0.mp4")
	require.Len(t, c.got(), 1)
}

func TestHandleRequestWithoutServeCounterStillServes(t *testing.T) {
	// The server hands the muxer no counter when it has no DVR service.
	up := &fakeUploader{uploaded: true, proxyOK: true}
	mi, _ := newServeMI(t, t.TempDir(), up, nil)

	rec := serve(mi, "seg0.mp4")

	require.Equal(t, "segment-bytes", rec.Body.String())
}

// ----- the counts reach the repository without DVR mode -----------------------

// segRepo is just enough of the segment repository for dvr.Service.RecordServe:
// the row lookup and the batched count write.
type segRepo struct {
	models.LiveHLSSegmentRepository

	mu     sync.Mutex
	rows   map[string]bool
	counts map[string][4]int64
}

func (r *segRepo) GetByStreamAndSegment(stream, segment string) (*models.LiveHLSSegment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.rows[stream+"/"+segment] {
		return nil, nil
	}

	return &models.LiveHLSSegment{StreamID: stream, SegmentName: segment}, nil
}

func (r *segRepo) AddServeCounts(_ context.Context, stream, segment string, local, redirect, proxy, miss int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := stream + "/" + segment
	c := r.counts[k]
	c[0] += local
	c[1] += redirect
	c[2] += proxy
	c[3] += miss
	r.counts[k] = c

	return nil
}

func (r *segRepo) get(key string) [4]int64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.counts[key]
}

func TestServeCountsAreWrittenThroughTheDVRServiceWithDVRDisabled(t *testing.T) {
	repo := &segRepo{
		rows:   map[string]bool{"stream-1/720/seg0.mp4": true},
		counts: map[string][4]int64{},
	}
	svc := &dvr.Service{Repository: repo}

	// DVREnabled is false: the server still gives its muxers the service.
	srv := &Server{DVRService: svc, DVREnabled: false}
	require.NotNil(t, srv.serveCounter())

	up := &fakeUploader{uploaded: true, proxyOK: true}
	mi, _ := newServeMI(t, t.TempDir(), up, srv.serveCounter())

	serve(mi, "seg0.mp4")
	serve(mi, "seg0.mp4")
	up.proxyOK = false
	up.presignURL = "https://cdn.example/x"
	serve(mi, "seg0.mp4")

	svc.Close() // flushes the pending counts

	// local, redirect, proxy, miss
	require.Equal(t, [4]int64{0, 1, 2, 0}, repo.get("stream-1/720/seg0.mp4"))
}

func TestServeCountsSkipASegmentWithoutARow(t *testing.T) {
	repo := &segRepo{rows: map[string]bool{}, counts: map[string][4]int64{}}
	svc := &dvr.Service{Repository: repo}
	mi, _ := newServeMI(t, "", &fakeUploader{}, svc)

	serve(mi, "unknown.mp4")
	svc.Close()

	require.Empty(t, repo.counts)
}

func TestServerWithoutDVRServiceHasNoServeCounter(t *testing.T) {
	// A nil *dvr.Service must not turn into a non-nil interface that panics.
	require.Nil(t, (&Server{}).serveCounter())
}
