package repository

import (
	"github.com/bluenviron/mediamtx/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type LiveWatchUsageRepository struct {
	db *gorm.DB
}

func NewLiveWatchUsageRepository(db *gorm.DB) models.LiveWatchUsageRepository {
	if db == nil {
		return nil
	}
	return &LiveWatchUsageRepository{db: db}
}

// AddWatchedMS adds deltaMS to the total with no memory of what was counted.
func (r *LiveWatchUsageRepository) AddWatchedMS(mediaID uuid.UUID, rendition string, deltaMS int64) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Exec(
		`INSERT INTO live_watch_usage (live_stream_media_id, rendition, watched_ms, billed_ms, updated_at)
VALUES (?, ?, ?, 0, now())
ON CONFLICT (live_stream_media_id, rendition) DO UPDATE
SET watched_ms = live_watch_usage.watched_ms + EXCLUDED.watched_ms, updated_at = now()`,
		mediaID, rendition, deltaMS,
	).Error
}

// AddSegmentWatch is one statement: the segment row is inserted first, and the
// total moves only when that insert happened, so a segment is counted once no
// matter how many viewers, requests or nodes report it.
func (r *LiveWatchUsageRepository) AddSegmentWatch(mediaID uuid.UUID, rendition, segment string, ms int64) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Exec(
		`WITH first_seen AS (
  INSERT INTO live_watch_segment (live_stream_media_id, rendition, segment_name, duration_ms)
  VALUES (?, ?, ?, ?)
  ON CONFLICT DO NOTHING
  RETURNING 1
)
INSERT INTO live_watch_usage (live_stream_media_id, rendition, watched_ms, billed_ms, updated_at)
SELECT ?, ?, ?, 0, now() WHERE EXISTS (SELECT 1 FROM first_seen)
ON CONFLICT (live_stream_media_id, rendition) DO UPDATE
SET watched_ms = live_watch_usage.watched_ms + EXCLUDED.watched_ms, updated_at = now()`,
		mediaID, rendition, segment, ms,
		mediaID, rendition, ms,
	).Error
}
