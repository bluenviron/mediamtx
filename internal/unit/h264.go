package unit

import (
	"time"
)

// H264 is a H264 data unit.
type H264 struct {
	Base
	PTS time.Duration
	AU  [][]byte
}
