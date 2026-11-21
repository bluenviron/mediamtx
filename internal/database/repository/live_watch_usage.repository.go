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
