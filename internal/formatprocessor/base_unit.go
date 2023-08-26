package formatprocessor

import (
	"time"

	"github.com/pion/rtp"
)

// BaseUnit contains fields shared across all units.
type BaseUnit struct {
	RTPPackets []*rtp.Packet
	NTP        time.Time
	PTS        time.Duration
}

// GetRTPPackets implements Unit.
func (u *BaseUnit) GetRTPPackets() []*rtp.Packet {
	return u.RTPPackets
}

// GetNTP implements Unit.
func (u *BaseUnit) GetNTP() time.Time {
	return u.NTP
}

// GetPTS implements Unit.
func (u *BaseUnit) GetPTS() time.Duration {
	return u.PTS
}
