// Package formatprocessor contains code to cleanup and normalize streams.
package formatprocessor

import (
	"time"

	"github.com/pion/rtp"

	"github.com/aler9/gortsplib/v2/pkg/format"
)

// Data is the elementary data unit routed across the server.
type Data interface {
	GetRTPPackets() []*rtp.Packet
	GetNTP() time.Time
}

// Processor allows to cleanup and normalize streams.
type Processor interface {
	// clears and normalizes a data unit.
	Process(Data, bool) error
}

// New allocates a Processor.
func New(forma format.Format, generateRTPPackets bool) (Processor, error) {
	switch forma := forma.(type) {
	case *format.H264:
		return newH264(forma, generateRTPPackets)

	case *format.H265:
		return newH265(forma, generateRTPPackets)

	case *format.VP8:
		return newVP8(forma, generateRTPPackets)

	case *format.VP9:
		return newVP9(forma, generateRTPPackets)

	case *format.MPEG4Audio:
		return newMPEG4Audio(forma, generateRTPPackets)

	case *format.Opus:
		return newOpus(forma, generateRTPPackets)

	default:
		return newGeneric(forma, generateRTPPackets)
	}
}
