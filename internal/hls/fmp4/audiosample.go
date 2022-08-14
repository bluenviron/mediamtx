package fmp4

import (
	"time"
)

// AudioSample is an audio sample.
type AudioSample struct {
	AU   []byte
	PTS  time.Duration
	Next *AudioSample
}

// Duration returns the sample duration.
func (s AudioSample) Duration() time.Duration {
	return s.Next.PTS - s.PTS
}
