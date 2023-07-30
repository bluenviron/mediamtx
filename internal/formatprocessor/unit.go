package formatprocessor

import (
	"time"

	"github.com/pion/rtp"
)

// BaseUnit contains fields shared across all units.
type BaseUnit struct {
	RTPPackets []*rtp.Packet
	NTP        time.Time
}

// GetRTPPackets implements Unit.
func (u *BaseUnit) GetRTPPackets() []*rtp.Packet {
	return u.RTPPackets
}

// GetNTP implements Unit.
func (u *BaseUnit) GetNTP() time.Time {
	return u.NTP
}

// Unit is the elementary data unit routed across the server.
type Unit interface {
	// returns RTP packets contained into the unit.
	GetRTPPackets() []*rtp.Packet

	// returns the NTP timestamp of the unit.
	GetNTP() time.Time
}
