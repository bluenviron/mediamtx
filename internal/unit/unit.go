// Package unit contains the unit definition.
package unit

import (
	"reflect"
	"time"

	"github.com/pion/rtp"
)

// Unit is an atomic unit of a stream.
type Unit struct {
	// relative time
	PTS int64

	// absolute time
	NTP time.Time

	// RTP packets
	RTPPackets []*rtp.Packet

	// codec-dependent payload
	Payload Payload
}

// NilPayload checks whether the payload is nil.
func (u Unit) NilPayload() bool {
	return u.Payload == nil || reflect.ValueOf(u.Payload).IsNil()
}
