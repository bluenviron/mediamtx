package repository

import (
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Needs MTX_TEST_POSTGRES_DSN. The tables come from the aioz-stream migrations
// 20261117000000_live_watch_usage and 20261123000000_live_watch_segment; they
// are created here only when missing.
func watchTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("MTX_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("MTX_TEST_POSTGRES_DSN not set")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS live_watch_usage (
		live_stream_media_id uuid NOT NULL, rendition text NOT NULL,
		watched_ms bigint NOT NULL DEFAULT 0, billed_ms bigint NOT NULL DEFAULT 0,
		updated_at timestamptz, PRIMARY KEY (live_stream_media_id, rendition))`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS live_watch_segment (
		live_stream_media_id uuid NOT NULL, rendition text NOT NULL, segment_name text NOT NULL,
		duration_ms bigint NOT NULL DEFAULT 0, created_at timestamptz NOT NULL DEFAULT now(),
		PRIMARY KEY (live_stream_media_id, rendition, segment_name))`).Error)

	return db
}

func watchedMS(t *testing.T, db *gorm.DB, id uuid.UUID, rendition string) int64 {
	t.Helper()

	var ms int64
	require.NoError(t, db.Raw(
		`SELECT COALESCE(SUM(watched_ms), 0) FROM live_watch_usage
		 WHERE live_stream_media_id = ? AND rendition = ?`, id, rendition,
	).Scan(&ms).Error)

	return ms
}

func TestAddSegmentWatchCountsASegmentOnce(t *testing.T) {
	db := watchTestDB(t)
	repo := NewLiveWatchUsageRepository(db)
	id := uuid.New()
	t.Cleanup(func() {
		db.Exec("DELETE FROM live_watch_usage WHERE live_stream_media_id = ?", id)
		db.Exec("DELETE FROM live_watch_segment WHERE live_stream_media_id = ?", id)
	})

	for range 5 {
		require.NoError(t, repo.AddSegmentWatch(id, "720", "seg0.mp4", 2000))
	}
	require.Equal(t, int64(2000), watchedMS(t, db, id, "720"))

	require.NoError(t, repo.AddSegmentWatch(id, "720", "seg1.mp4", 2000))
	require.Equal(t, int64(4000), watchedMS(t, db, id, "720"))

	// The same segment name under another rendition is a different segment.
	require.NoError(t, repo.AddSegmentWatch(id, "480", "seg0.mp4", 2000))
	require.Equal(t, int64(2000), watchedMS(t, db, id, "480"))
	require.Equal(t, int64(4000), watchedMS(t, db, id, "720"))
}

func TestAddSegmentWatchIsOnceUnderConcurrency(t *testing.T) {
	db := watchTestDB(t)
	repo := NewLiveWatchUsageRepository(db)
	id := uuid.New()
	t.Cleanup(func() {
		db.Exec("DELETE FROM live_watch_usage WHERE live_stream_media_id = ?", id)
		db.Exec("DELETE FROM live_watch_segment WHERE live_stream_media_id = ?", id)
	})

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			require.NoError(t, repo.AddSegmentWatch(id, "1080", "seg0.mp4", 2000))
		}()
	}
	wg.Wait()

	require.Equal(t, int64(2000), watchedMS(t, db, id, "1080"))
}
