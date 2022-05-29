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
	writeH264(pts time.Duration, nalus [][]byte) error
	writeAAC(pts time.Duration, aus [][]byte) error
	file(name string, msn string, part string, skip string) *MuxerFileResponse
}
