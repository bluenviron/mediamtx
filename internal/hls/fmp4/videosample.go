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
	AVCC       []byte
	IDRPresent bool
	Next       *VideoSample
}

// Duration returns the sample duration.
func (s VideoSample) Duration() time.Duration {
	return s.Next.DTS - s.DTS
}
