// Package formatprocessor contains code to cleanup and normalize streams.
package formatprocessor

import (
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/unit"
)

// avoid an int64 overflow and preserve resolution by splitting division into two parts:
// first add the integer part, then the decimal part.
func multiplyAndDivide(v, m, d time.Duration) time.Duration {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

func setTimestamp(newPackets []*rtp.Packet, oldPackets []*rtp.Packet, clockRate int, pts time.Duration) {
	if oldPackets != nil { // get timestamp from old packets
		for _, pkt := range newPackets {
			pkt.Timestamp = oldPackets[0].Timestamp
		}
	} else { // get timestamp from PTS
		for _, pkt := range newPackets {
			pkt.Timestamp = uint32(multiplyAndDivide(pts, time.Duration(clockRate), time.Second))
		}
	}
}

// Processor cleans and normalizes streams.
type Processor interface {
	// cleans and normalizes a data unit.
	Process(unit.Unit, bool) error

	// wraps a RTP packet into a Unit.
	UnitForRTPPacket(pkt *rtp.Packet, ntp time.Time, pts time.Duration) Unit
}

// New allocates a Processor.
func New(
	udpMaxPayloadSize int,
	forma format.Format,
	generateRTPPackets bool,
) (Processor, error) {
	switch forma := forma.(type) {
	case *format.H264:
		return newH264(udpMaxPayloadSize, forma, generateRTPPackets)

	case *format.H265:
		return newH265(udpMaxPayloadSize, forma, generateRTPPackets)

	case *format.VP8:
		return newVP8(udpMaxPayloadSize, forma, generateRTPPackets)

	case *format.VP9:
		return newVP9(udpMaxPayloadSize, forma, generateRTPPackets)

	case *format.AV1:
		return newAV1(udpMaxPayloadSize, forma, generateRTPPackets)

	case *format.MPEG1Audio:
		return newMPEG1Audio(udpMaxPayloadSize, forma, generateRTPPackets)

	case *format.MPEG4AudioGeneric:
		return newMPEG4AudioGeneric(udpMaxPayloadSize, forma, generateRTPPackets)

	case *format.MPEG4AudioLATM:
		return newMPEG4AudioLATM(udpMaxPayloadSize, forma, generateRTPPackets)

	case *format.Opus:
		return newOpus(udpMaxPayloadSize, forma, generateRTPPackets)

	default:
		return newGeneric(udpMaxPayloadSize, forma, generateRTPPackets)
	}
}
