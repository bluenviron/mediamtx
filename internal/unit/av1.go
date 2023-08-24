package unit

import (
	"time"
)

// AV1 is an AV1 data unit.
type AV1 struct {
	Base
	PTS time.Duration
	TU  [][]byte
}
