// Package formatprocessor contains code to cleanup and normalize streams.
package formatprocessor

import (
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
)

// Processor allows to cleanup and normalize streams.
type Processor interface {
	// clears and normalizes a data unit.
	Process(Unit, bool) error
}

// New allocates a Processor.
func New(
	udpMaxPayloadSize int,
	forma formats.Format,
	generateRTPPackets bool,
) (Processor, error) {
	switch forma := forma.(type) {
	case *formats.H264:
		return newH264(udpMaxPayloadSize, forma, generateRTPPackets)

	case *formats.H265:
		return newH265(udpMaxPayloadSize, forma, generateRTPPackets)

	case *formats.VP8:
		return newVP8(udpMaxPayloadSize, forma, generateRTPPackets)

	case *formats.VP9:
		return newVP9(udpMaxPayloadSize, forma, generateRTPPackets)

	case *formats.MPEG2Audio:
		return newMPEG2Audio(udpMaxPayloadSize, forma, generateRTPPackets)

	case *formats.MPEG4Audio:
		return newMPEG4Audio(udpMaxPayloadSize, forma, generateRTPPackets)

	case *formats.Opus:
		return newOpus(udpMaxPayloadSize, forma, generateRTPPackets)

	default:
		return newGeneric(udpMaxPayloadSize, forma, generateRTPPackets)
	}
}
