// Package formatprocessor cleans and normalizes streams.
package formatprocessor

import (
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/unit"
)

// Processor cleans and normalizes streams.
type Processor interface {
	// process a Unit.
	ProcessUnit(unit.Unit) error

	// process a RTP packet and convert it into a unit.
	ProcessRTPPacket(
		pkt *rtp.Packet,
		ntp time.Time,
		pts int64,
		hasNonRTSPReaders bool,
	) (unit.Unit, error)
}

// New allocates a Processor.
func New(
	udpMaxPayloadSize int,
	forma format.Format,
	generateRTPPackets bool,
) (Processor, error) {
	switch forma := forma.(type) {
	case *format.AV1:
		return newAV1(udpMaxPayloadSize, forma, generateRTPPackets)

	case *format.VP9:
		return newVP9(udpMaxPayloadSize, forma, generateRTPPackets)

	case *format.VP8:
		return newVP8(udpMaxPayloadSize, forma, generateRTPPackets)

	case *format.H265:
		return newH265(udpMaxPayloadSize, forma, generateRTPPackets)

	case *format.H264:
		return newH264(udpMaxPayloadSize, forma, generateRTPPackets)

	case *format.MPEG4Video:
		return newMPEG4Video(udpMaxPayloadSize, forma, generateRTPPackets)

	case *format.MPEG1Video:
		return newMPEG1Video(udpMaxPayloadSize, forma, generateRTPPackets)

	case *format.Opus:
		return newOpus(udpMaxPayloadSize, forma, generateRTPPackets)

	case *format.MPEG4Audio:
		return newMPEG4Audio(udpMaxPayloadSize, forma, generateRTPPackets)

	case *format.MPEG1Audio:
		return newMPEG1Audio(udpMaxPayloadSize, forma, generateRTPPackets)

	case *format.MJPEG:
		return newMJPEG(udpMaxPayloadSize, forma, generateRTPPackets)

	case *format.AC3:
		return newAC3(udpMaxPayloadSize, forma, generateRTPPackets)

	case *format.G711:
		return newG711(udpMaxPayloadSize, forma, generateRTPPackets)

	case *format.LPCM:
		return newLPCM(udpMaxPayloadSize, forma, generateRTPPackets)

	default:
		return newGeneric(udpMaxPayloadSize, forma, generateRTPPackets)
	}
}
