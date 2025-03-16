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

	initialize() error
}

// New allocates a Processor.
func New(
	udpMaxPayloadSize int,
	forma format.Format,
	generateRTPPackets bool,
) (Processor, error) {
	var proc Processor

	switch forma := forma.(type) {
	case *format.AV1:
		proc = &formatProcessorAV1{
			UDPMaxPayloadSize:  udpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
		}

	case *format.VP9:
		proc = &formatProcessorVP9{
			UDPMaxPayloadSize:  udpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
		}

	case *format.VP8:
		proc = &formatProcessorVP8{
			UDPMaxPayloadSize:  udpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
		}

	case *format.H265:
		proc = &formatProcessorH265{
			UDPMaxPayloadSize:  udpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
		}

	case *format.H264:
		proc = &formatProcessorH264{
			UDPMaxPayloadSize:  udpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
		}

	case *format.MPEG4Video:
		proc = &formatProcessorMPEG4Video{
			UDPMaxPayloadSize:  udpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
		}

	case *format.MPEG1Video:
		proc = &formatProcessorMPEG1Video{
			UDPMaxPayloadSize:  udpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
		}

	case *format.Opus:
		proc = &formatProcessorOpus{
			UDPMaxPayloadSize:  udpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
		}

	case *format.MPEG4Audio:
		proc = &formatProcessorMPEG4Audio{
			UDPMaxPayloadSize:  udpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
		}

	case *format.MPEG1Audio:
		proc = &formatProcessorMPEG1Audio{
			UDPMaxPayloadSize:  udpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
		}

	case *format.MJPEG:
		proc = &formatProcessorMJPEG{
			UDPMaxPayloadSize:  udpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
		}

	case *format.AC3:
		proc = &formatProcessorAC3{
			UDPMaxPayloadSize:  udpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
		}

	case *format.G711:
		proc = &formatProcessorG711{
			UDPMaxPayloadSize:  udpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
		}

	case *format.LPCM:
		proc = &formatProcessorLPCM{
			UDPMaxPayloadSize:  udpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
		}

	default:
		proc = &formatProcessorGeneric{
			UDPMaxPayloadSize:  udpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
		}
	}

	err := proc.initialize()
	return proc, err
}
