// Package formatprocessor contains code to cleanup and normalize streams.
package formatprocessor

import (
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
)

// Processor cleans and normalizes streams.
type Processor interface {
	// cleans and normalizes a data unit.
	Process(Unit, bool) error

	// wraps a RTP packet into a Unit.
	UnitForRTPPacket(pkt *rtp.Packet, ntp time.Time) Unit
}

// New allocates a Processor.
func New(
	udpMaxPayloadSize int,
	forma formats.Format,
	generateRTPPackets bool,
	log logger.Writer,
) (Processor, error) {
	switch forma := forma.(type) {
	case *formats.H264:
		return newH264(udpMaxPayloadSize, forma, generateRTPPackets, log)

	case *formats.H265:
		return newH265(udpMaxPayloadSize, forma, generateRTPPackets, log)

	case *formats.VP8:
		return newVP8(udpMaxPayloadSize, forma, generateRTPPackets, log)

	case *formats.VP9:
		return newVP9(udpMaxPayloadSize, forma, generateRTPPackets, log)

	case *formats.AV1:
		return newAV1(udpMaxPayloadSize, forma, generateRTPPackets, log)

	case *formats.MPEG1Audio:
		return newMPEG1Audio(udpMaxPayloadSize, forma, generateRTPPackets, log)

	case *formats.MPEG4AudioGeneric:
		return newMPEG4AudioGeneric(udpMaxPayloadSize, forma, generateRTPPackets, log)

	case *formats.MPEG4AudioLATM:
		return newMPEG4AudioLATM(udpMaxPayloadSize, forma, generateRTPPackets, log)

	case *formats.Opus:
		return newOpus(udpMaxPayloadSize, forma, generateRTPPackets, log)

	default:
		return newGeneric(udpMaxPayloadSize, forma, generateRTPPackets, log)
	}
}
