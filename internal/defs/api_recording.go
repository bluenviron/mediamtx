package defs

import "time"

// APIRecordingSegment is a recording segment.
type APIRecordingSegment struct {
	Start time.Time `json:"start"`
	// Duration in seconds. Null when unavailable or when the format is MPEG-TS.
	Duration *float64 `json:"duration"`
}

// APIRecording is a recording.
type APIRecording struct {
	Name     string                `json:"name"`
	Segments []APIRecordingSegment `json:"segments"`
}

// APIRecordingList is a list of recordings.
type APIRecordingList struct {
	ItemCount int            `json:"itemCount"`
	PageCount int            `json:"pageCount"`
	Items     []APIRecording `json:"items"`
}
