package fmp4

import (
	"time"
)

const (
	videoTimescale = 90000
)

// VideoSample is a video sample.
type VideoSample struct {
	NALUs      [][]byte
	PTS        time.Duration
	DTS        time.Duration
	IDRPresent bool
	Next       *VideoSample

	avcc []byte
}

// Duration returns the sample duration.
func (s VideoSample) Duration() time.Duration {
	return s.Next.DTS - s.DTS
}
