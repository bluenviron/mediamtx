package watchusage

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type call struct {
	id  uuid.UUID
	r   string
	seg string
	ms  int64
}

type fakeRepo struct {
	mu    sync.Mutex
	calls []call
	fail  bool
}

func (f *fakeRepo) AddWatchedMS(id uuid.UUID, r string, ms int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("boom")
	}
	f.calls = append(f.calls, call{id: id, r: r, ms: ms})
	return nil
}

func (f *fakeRepo) AddSegmentWatch(id uuid.UUID, r, seg string, ms int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("boom")
	}
	f.calls = append(f.calls, call{id, r, seg, ms})
	return nil
}

func TestRenditionFromPath(t *testing.T) {
	id := uuid.New()
	for _, tc := range []struct {
		path string
		r    string
		ok   bool
	}{
		{id.String() + "/720", "720", true},
		{id.String() + "/video/1080", "1080", true},
		{id.String() + "/original", "", false},
		{"live/720", "", false},
		{"", "", false},
	} {
		got, r, ok := RenditionFromPath(tc.path)
		require.Equal(t, tc.ok, ok, tc.path)
		if ok {
			require.Equal(t, id, got)
			require.Equal(t, tc.r, r)
		}
	}
}

func TestBillable(t *testing.T) {
	require.True(t, Billable("480"))
	require.True(t, Billable("720"))
	require.True(t, Billable("1080"))
	require.False(t, Billable("360"))
	require.False(t, Billable("original"))
}

func TestCounterCollapsesRepeatsOfOneSegment(t *testing.T) {
	repo := &fakeRepo{}
	c := New(repo, time.Hour)
	id := uuid.New()
	c.Add(id, "720", "seg0.mp4", 2000)
	c.Add(id, "720", "seg0.mp4", 2000)
	c.Add(id, "720", "seg0.mp4", 2000)
	c.Add(id, "720", "seg1.mp4", 2000)
	c.Add(id, "480", "seg0.mp4", 2000)
	c.Add(id, "360", "seg0.mp4", 2000)
	c.Add(id, "720", "", 2000)
	c.Add(id, "720", "seg2.mp4", 0)
	c.Add(id, "1080", "seg2.mp4", -5)
	c.Close(context.Background())
	require.ElementsMatch(t, []call{
		{id, "720", "seg0.mp4", 2000},
		{id, "720", "seg1.mp4", 2000},
		{id, "480", "seg0.mp4", 2000},
	}, repo.calls)
}

func TestCounterRetriesFailedFlushWithoutDoubleCounting(t *testing.T) {
	repo := &fakeRepo{fail: true}
	c := New(repo, time.Hour)
	id := uuid.New()
	c.Add(id, "1080", "seg0.mp4", 2000)
	c.flush()
	require.Empty(t, repo.calls)
	repo.fail = false
	c.Add(id, "1080", "seg0.mp4", 2000)
	c.Close(context.Background())
	require.Equal(t, []call{{id, "1080", "seg0.mp4", 2000}}, repo.calls)
}

func TestCounterPerRequestAddsEveryRequest(t *testing.T) {
	repo := &fakeRepo{}
	c := New(repo, time.Hour, WithBillOnce(false))
	id := uuid.New()
	c.Add(id, "720", "seg0.mp4", 2000)
	c.Add(id, "720", "seg0.mp4", 2000)
	c.Add(id, "720", "seg0.mp4", 2000)
	c.Add(id, "720", "seg1.mp4", 2000)
	c.Add(id, "480", "seg0.mp4", 2000)
	c.Add(id, "360", "seg0.mp4", 2000)
	c.Add(id, "720", "seg2.mp4", 0)
	c.Close(context.Background())
	// One total per (stream, rendition); the segment name plays no part.
	require.ElementsMatch(t, []call{
		{id: id, r: "720", ms: 8000},
		{id: id, r: "480", ms: 2000},
	}, repo.calls)
}

func TestCounterPerRequestKeepsDeltaOnFailure(t *testing.T) {
	repo := &fakeRepo{fail: true}
	c := New(repo, time.Hour, WithBillOnce(false))
	id := uuid.New()
	c.Add(id, "1080", "seg0.mp4", 2000)
	c.flush()
	require.Empty(t, repo.calls)
	repo.fail = false
	c.Add(id, "1080", "seg1.mp4", 1000)
	c.Close(context.Background())
	require.Equal(t, []call{{id: id, r: "1080", ms: 3000}}, repo.calls)
}

func TestCounterBillsOnceByDefault(t *testing.T) {
	repo := &fakeRepo{}
	c := New(repo, time.Hour)
	id := uuid.New()
	c.Add(id, "720", "seg0.mp4", 2000)
	c.Add(id, "720", "seg0.mp4", 2000)
	c.Close(context.Background())
	require.Equal(t, []call{{id, "720", "seg0.mp4", 2000}}, repo.calls)
}
