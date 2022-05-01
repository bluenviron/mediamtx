package hls

import (
	"io"
	"time"
)

// MuxerVariant is a muxer variant.
type MuxerVariant int

// supported variants.
const (
	MuxerVariantLowLatency MuxerVariant = iota
	MuxerVariantMPEGTS
	MuxerVariantFMP4
)

type muxerVariant interface {
	close()
	writeH264(pts time.Duration, nalus [][]byte) error
	writeAAC(pts time.Duration, aus [][]byte) error
	playlistReader(msn string, part string, skip string) io.Reader
	segmentReader(string) io.Reader
}
