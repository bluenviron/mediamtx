package unit

import (
	"time"
)

// VP8 is a VP8 data unit.
type VP8 struct {
	Base
	PTS   time.Duration
	Frame []byte
}
