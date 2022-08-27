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
	writeH264(now time.Time, pts time.Duration, nalus [][]byte) error
	writeAAC(now time.Time, pts time.Duration, au []byte) error
	file(name string, msn string, part string, skip string) *MuxerFileResponse
}
