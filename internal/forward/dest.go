package forward

import (
	"context"

	"github.com/bluenviron/mediamtx/internal/stream"
)

// Dest is a protocol-specific forward destination.
type Dest interface {
	Run(context.Context, *stream.Stream) error
	OutboundBytes() uint64
}
