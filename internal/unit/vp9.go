package unit

import (
	"time"
)

// VP9 is a VP9 data unit.
type VP9 struct {
	Base
	PTS   time.Duration
	Frame []byte
}
