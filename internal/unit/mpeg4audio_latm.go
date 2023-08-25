package unit

import (
	"time"
)

// MPEG4AudioLATM is a MPEG-4 Audio data unit.
type MPEG4AudioLATM struct {
	Base
	PTS time.Duration
	AU  []byte
}
