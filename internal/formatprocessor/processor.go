// Package formatprocessor contains code to cleanup and normalize streams.
package formatprocessor

import (
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"

	"github.com/aler9/mediamtx/internal/logger"
)

const (
	maxKeyFrameInterval = 10 * time.Second
)

// Processor cleans and normalizes streams.
type Processor interface {
	// cleans and normalizes a data unit.
	Process(Unit, bool) error
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

	case *formats.MPEG2Audio:
		return newMPEG2Audio(udpMaxPayloadSize, forma, generateRTPPackets, log)

	case *formats.MPEG4Audio:
		return newMPEG4Audio(udpMaxPayloadSize, forma, generateRTPPackets, log)

	case *formats.Opus:
		return newOpus(udpMaxPayloadSize, forma, generateRTPPackets, log)

	default:
		return newGeneric(udpMaxPayloadSize, forma, generateRTPPackets, log)
	}
}
