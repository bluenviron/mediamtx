// Package hls contains a HLS muxer and client.
package hls

import (
	"io"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"
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
	videoTrack format.Format,
	audioTrack format.Format,
) (*Muxer, error) {
	m := &Muxer{}

	switch variant {
	case MuxerVariantMPEGTS:
		var err error
		m.variant, err = newMuxerVariantMPEGTS(
			segmentCount,
			segmentDuration,
			segmentMaxSize,
			videoTrack,
			audioTrack,
		)
		if err != nil {
			return nil, err
		}

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

// WriteH26x writes an H264 or an H265 access unit.
func (m *Muxer) WriteH26x(ntp time.Time, pts time.Duration, au [][]byte) error {
	return m.variant.writeH26x(ntp, pts, au)
}

// WriteAudio writes an audio access unit.
func (m *Muxer) WriteAudio(ntp time.Time, pts time.Duration, au []byte) error {
	return m.variant.writeAudio(ntp, pts, au)
}

// File returns a file reader.
func (m *Muxer) File(name string, msn string, part string, skip string) *MuxerFileResponse {
	if name == "index.m3u8" {
		return m.primaryPlaylist.file()
	}

	return m.variant.file(name, msn, part, skip)
}
