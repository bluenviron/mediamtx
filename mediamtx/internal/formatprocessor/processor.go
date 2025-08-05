// Package formatprocessor cleans and normalizes streams.
package formatprocessor

import (
	"crypto/rand"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func randUint32() (uint32, error) {
	var b [4]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return 0, err
	}
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]), nil
}

// Processor is the codec-dependent part of the processing that happens inside stream.Stream.
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
	rtpMaxPayloadSize int,
	forma format.Format,
	generateRTPPackets bool,
	parent logger.Writer,
) (Processor, error) {
	var proc Processor

	switch forma := forma.(type) {
	case *format.AV1:
		proc = &av1{
			RTPMaxPayloadSize:  rtpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
			Parent:             parent,
		}

	case *format.VP9:
		proc = &vp9{
			RTPMaxPayloadSize:  rtpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
			Parent:             parent,
		}

	case *format.VP8:
		proc = &vp8{
			RTPMaxPayloadSize:  rtpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
			Parent:             parent,
		}

	case *format.H265:
		proc = &h265{
			RTPMaxPayloadSize:  rtpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
			Parent:             parent,
		}

	case *format.H264:
		proc = &h264{
			RTPMaxPayloadSize:  rtpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
			Parent:             parent,
		}

	case *format.MPEG4Video:
		proc = &mpeg4Video{
			RTPMaxPayloadSize:  rtpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
			Parent:             parent,
		}

	case *format.MPEG1Video:
		proc = &mpeg1Video{
			RTPMaxPayloadSize:  rtpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
			Parent:             parent,
		}

	case *format.MJPEG:
		proc = &mjpeg{
			RTPMaxPayloadSize:  rtpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
			Parent:             parent,
		}

	case *format.Opus:
		proc = &opus{
			RTPMaxPayloadSize:  rtpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
			Parent:             parent,
		}

	case *format.KLV:
		proc = &klv{
			RTPMaxPayloadSize:  rtpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
			Parent:             parent,
		}

	case *format.MPEG4Audio:
		proc = &mpeg4Audio{
			RTPMaxPayloadSize:  rtpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
			Parent:             parent,
		}

	case *format.MPEG4AudioLATM:
		proc = &mpeg4AudioLATM{
			RTPMaxPayloadSize:  rtpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
			Parent:             parent,
		}

	case *format.MPEG1Audio:
		proc = &mpeg1Audio{
			RTPMaxPayloadSize:  rtpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
			Parent:             parent,
		}

	case *format.AC3:
		proc = &ac3{
			RTPMaxPayloadSize:  rtpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
			Parent:             parent,
		}

	case *format.G711:
		proc = &g711{
			RTPMaxPayloadSize:  rtpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
			Parent:             parent,
		}

	case *format.LPCM:
		proc = &lpcm{
			RTPMaxPayloadSize:  rtpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
			Parent:             parent,
		}

	default:
		proc = &generic{
			RTPMaxPayloadSize:  rtpMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: generateRTPPackets,
			Parent:             parent,
		}
	}

	err := proc.initialize()
	return proc, err
}
