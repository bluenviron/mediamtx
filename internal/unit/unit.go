// Package unit contains the Unit definition.
package unit

import (
	"time"

	"github.com/pion/rtp"
)

// Unit is the elementary data unit routed across the server.
type Unit interface {
	// returns RTP packets contained into the unit.
	GetRTPPackets() []*rtp.Packet

	// returns the NTP timestamp of the unit.
	GetNTP() time.Time

	// returns the PTS of the unit.
	GetPTS() int64
}
