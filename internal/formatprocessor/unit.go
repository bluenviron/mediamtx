package formatprocessor

import (
	"time"

	"github.com/pion/rtp"
)

// Unit is the elementary data unit routed across the server.
type Unit interface {
	GetRTPPackets() []*rtp.Packet
	GetNTP() time.Time
}
