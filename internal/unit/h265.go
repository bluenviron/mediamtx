package unit

import (
	"time"
)

// H265 is a H265 data unit.
type H265 struct {
	Base
	PTS time.Duration
	AU  [][]byte
}
