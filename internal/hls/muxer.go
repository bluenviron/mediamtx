package hls

import (
	"io"
	"time"

	"github.com/aler9/gortsplib"
)

// Muxer is a HLS muxer.
type Muxer struct {
	primaryPlaylist *muxerPrimaryPlaylist
	streamPlaylist  *muxerStreamPlaylist
	tsGenerator     *muxerTSGenerator
}

// NewMuxer allocates a Muxer.
func NewMuxer(
	hlsSegmentCount int,
	hlsSegmentDuration time.Duration,
	videoTrack *gortsplib.Track,
	audioTrack *gortsplib.Track) (*Muxer, error) {
	var h264Conf *gortsplib.TrackConfigH264
	if videoTrack != nil {
		var err error
		h264Conf, err = videoTrack.ExtractConfigH264()
		if err != nil {
			return nil, err
		}
	}

	var aacConf *gortsplib.TrackConfigAAC
	if audioTrack != nil {
		var err error
		aacConf, err = audioTrack.ExtractConfigAAC()
		if err != nil {
			return nil, err
		}
	}

	primaryPlaylist := newMuxerPrimaryPlaylist(videoTrack, audioTrack, h264Conf)

	streamPlaylist := newMuxerStreamPlaylist(hlsSegmentCount)

	tsGenerator := newMuxerTSGenerator(
		hlsSegmentCount,
		hlsSegmentDuration,
		videoTrack,
		audioTrack,
		h264Conf,
		aacConf,
		streamPlaylist)

	m := &Muxer{
		primaryPlaylist: primaryPlaylist,
		streamPlaylist:  streamPlaylist,
		tsGenerator:     tsGenerator,
	}

	return m, nil
}

// Close closes a Muxer.
func (m *Muxer) Close() {
	m.streamPlaylist.close()
}

// WriteH264 writes H264 NALUs, grouped by PTS, into the muxer.
func (m *Muxer) WriteH264(pts time.Duration, nalus [][]byte) error {
	return m.tsGenerator.writeH264(pts, nalus)
}

// WriteAAC writes AAC AUs, grouped by PTS, into the muxer.
func (m *Muxer) WriteAAC(pts time.Duration, aus [][]byte) error {
	return m.tsGenerator.writeAAC(pts, aus)
}

// PrimaryPlaylist returns a reader to read the primary playlist.
func (m *Muxer) PrimaryPlaylist() io.Reader {
	return m.primaryPlaylist.reader()
}

// StreamPlaylist returns a reader to read the stream playlist.
func (m *Muxer) StreamPlaylist() io.Reader {
	return m.streamPlaylist.reader()
}

// Segment returns a reader to read a segment listed in the stream playlist.
func (m *Muxer) Segment(fname string) io.Reader {
	return m.streamPlaylist.segment(fname)
}
