package unit

import (
	"time"
)

// Opus is a Opus data unit.
type Opus struct {
	Base
	PTS     time.Duration
	Packets [][]byte
}
