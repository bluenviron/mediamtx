package hls

import (
	"time"
)

// MuxerVariant is a muxer variant.
type MuxerVariant int

// supported variants.
const (
	MuxerVariantMPEGTS MuxerVariant = iota
	MuxerVariantFMP4
	MuxerVariantLowLatency
)

type muxerVariant interface {
	close()
	writeH26x(time.Time, time.Duration, [][]byte) error
	writeAudio(time.Time, time.Duration, []byte) error
	file(name string, msn string, part string, skip string) *MuxerFileResponse
}
