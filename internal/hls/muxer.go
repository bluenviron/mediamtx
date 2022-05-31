package hls

import (
	"io"
	"time"

	"github.com/aler9/gortsplib"
)

// MuxerFileResponse is a response of the Muxer's File() func.
type MuxerFileResponse struct {
	Status int
	Header map[string]string
	Body   io.Reader
}

// Muxer is a HLS muxer.
type Muxer struct {
	primaryPlaylist *muxerPrimaryPlaylist
	variant         muxerVariant
}

// NewMuxer allocates a Muxer.
func NewMuxer(
	variant MuxerVariant,
	segmentCount int,
	segmentDuration time.Duration,
	partDuration time.Duration,
	segmentMaxSize uint64,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
) (*Muxer, error) {
	m := &Muxer{}

	switch variant {
	case MuxerVariantMPEGTS:
		m.variant = newMuxerVariantMPEGTS(
			segmentCount,
			segmentDuration,
			segmentMaxSize,
			videoTrack,
			audioTrack,
		)

	case MuxerVariantFMP4:
		m.variant = newMuxerVariantFMP4(
			false,
			segmentCount,
			segmentDuration,
			partDuration,
			segmentMaxSize,
			videoTrack,
			audioTrack,
		)

	default: // MuxerVariantLowLatency
		m.variant = newMuxerVariantFMP4(
			true,
			segmentCount,
			segmentDuration,
			partDuration,
			segmentMaxSize,
			videoTrack,
			audioTrack,
		)
	}

	m.primaryPlaylist = newMuxerPrimaryPlaylist(variant != MuxerVariantMPEGTS, videoTrack, audioTrack)

	return m, nil
}

// Close closes a Muxer.
func (m *Muxer) Close() {
	m.variant.close()
}

// WriteH264 writes H264 NALUs, grouped by timestamp.
func (m *Muxer) WriteH264(pts time.Duration, nalus [][]byte) error {
	return m.variant.writeH264(pts, nalus)
}

// WriteAAC writes AAC AUs, grouped by timestamp.
func (m *Muxer) WriteAAC(pts time.Duration, aus [][]byte) error {
	return m.variant.writeAAC(pts, aus)
}

// File returns a file reader.
func (m *Muxer) File(name string, msn string, part string, skip string) *MuxerFileResponse {
	if name == "index.m3u8" {
		return m.primaryPlaylist.file()
	}

	return m.variant.file(name, msn, part, skip)
}
