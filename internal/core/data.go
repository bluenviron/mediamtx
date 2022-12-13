package core

import (
	"time"

	"github.com/pion/rtp"
)

// data is the data unit routed across the server.
type data interface {
	getRTPPackets() []*rtp.Packet
	getNTP() time.Time
}
