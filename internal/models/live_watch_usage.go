package models

import (
	"time"

	"github.com/google/uuid"
)

// LiveWatchUsage accumulates watched milliseconds per (media, rendition).
// mediamtx only increments WatchedMS; the API advances BilledMS.
type LiveWatchUsage struct {
	LiveStreamMediaID uuid.UUID `gorm:"column:live_stream_media_id;type:uuid;primaryKey"`
	Rendition         string    `gorm:"column:rendition;primaryKey"`
	WatchedMS         int64     `gorm:"column:watched_ms"`
	BilledMS          int64     `gorm:"column:billed_ms"`
	UpdatedAt         time.Time `gorm:"column:updated_at"`
}

func (LiveWatchUsage) TableName() string {
	return "live_watch_usage"
}

type LiveWatchUsageRepository interface {
	// AddWatchedMS adds deltaMS to the (media, rendition) total unconditionally:
	// every request counts.
	AddWatchedMS(mediaID uuid.UUID, rendition string, deltaMS int64) error
	// AddSegmentWatch adds ms to the (media, rendition) total only the first
	// time segment is seen; a repeat of the same segment changes nothing.
	AddSegmentWatch(mediaID uuid.UUID, rendition, segment string, ms int64) error
}
