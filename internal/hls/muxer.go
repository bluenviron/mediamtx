package hls

import (
	"io"
	"time"

	"github.com/aler9/gortsplib"
)

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
	segmentMaxSize uint64,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
) (*Muxer, error) {
	m := &Muxer{}

	var version int
	switch variant {
	case MuxerVariantMPEGTS:
		m.variant = newMuxerVariantMPEGTS(
			segmentCount,
			segmentDuration,
			segmentMaxSize,
			videoTrack,
			audioTrack,
		)
		version = 3

	case MuxerVariantFMP4:
		m.variant = newMuxerVariantFMP4(
			false,
			segmentCount,
			segmentDuration,
			segmentMaxSize,
			videoTrack,
			audioTrack,
		)
		version = 7

	default: // MuxerVariantLowLatency
		m.variant = newMuxerVariantFMP4(
			true,
			segmentCount,
			segmentDuration,
			segmentMaxSize,
			videoTrack,
			audioTrack,
		)
		version = 7
	}

	m.primaryPlaylist = newMuxerPrimaryPlaylist(version, videoTrack, audioTrack)

	return m, nil
}

// Close closes a Muxer.
func (m *Muxer) Close() {
	m.variant.close()
}

// PrimaryPlaylistReader returns a reader to read the primary playlist.
func (m *Muxer) PrimaryPlaylistReader() io.Reader {
	return m.primaryPlaylist.reader()
}

// WriteH264 writes H264 NALUs, grouped by timestamp.
func (m *Muxer) WriteH264(pts time.Duration, nalus [][]byte) error {
	return m.variant.writeH264(pts, nalus)
}

// WriteAAC writes AAC AUs, grouped by timestamp.
func (m *Muxer) WriteAAC(pts time.Duration, aus [][]byte) error {
	return m.variant.writeAAC(pts, aus)
}

// PlaylistReader returns a reader to read the stream playlist.
func (m *Muxer) PlaylistReader(msn string, part string, skip string) io.Reader {
	return m.variant.playlistReader(msn, part, skip)
}

// SegmentReader returns a reader to read a segment listed in the stream playlist.
func (m *Muxer) SegmentReader(fname string) io.Reader {
	return m.variant.segmentReader(fname)
}
