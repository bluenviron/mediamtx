package unit

import (
	"time"

	"github.com/pion/rtp"
)

// Base contains fields shared across all units.
type Base struct {
	RTPPackets []*rtp.Packet
	NTP        time.Time
	PTS        int64
}

// GetRTPPackets implements Unit.
func (u *Base) GetRTPPackets() []*rtp.Packet {
	return u.RTPPackets
}

// GetNTP implements Unit.
func (u *Base) GetNTP() time.Time {
	return u.NTP
}

// GetPTS implements Unit.
func (u *Base) GetPTS() int64 {
	return u.PTS
}
