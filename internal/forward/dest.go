package forward

import (
	"context"
)

// Dest is a protocol-specific forward destination.
type Dest interface {
	Run(context.Context) error
	OutboundBytes() uint64
}
