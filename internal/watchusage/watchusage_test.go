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
	id uuid.UUID
	r  string
	ms int64
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
	f.calls = append(f.calls, call{id, r, ms})
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
	require.True(t, Billable("720"))
	require.True(t, Billable("1080"))
	require.False(t, Billable("480"))
	require.False(t, Billable("original"))
}

func TestCounterAggregates(t *testing.T) {
	repo := &fakeRepo{}
	c := New(repo, time.Hour)
	id := uuid.New()
	c.Add(id, "720", 2000)
	c.Add(id, "720", 2000)
	c.Add(id, "720", 1000)
	c.Add(id, "480", 5000)
	c.Add(id, "720", 0)
	c.Add(id, "1080", -5)
	c.Close(context.Background())
	require.Equal(t, []call{{id, "720", 5000}}, repo.calls)
}

func TestCounterKeepsDeltaOnFailure(t *testing.T) {
	repo := &fakeRepo{fail: true}
	c := New(repo, time.Hour)
	id := uuid.New()
	c.Add(id, "1080", 2000)
	c.flush()
	require.Empty(t, repo.calls)
	repo.fail = false
	c.Add(id, "1080", 1000)
	c.Close(context.Background())
	require.Equal(t, []call{{id, "1080", 3000}}, repo.calls)
}
