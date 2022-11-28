package hls

import (
	"time"

	"github.com/aler9/gortsplib"
)

type muxerVariantMPEGTS struct {
	playlist  *muxerVariantMPEGTSPlaylist
	segmenter *muxerVariantMPEGTSSegmenter
}

func newMuxerVariantMPEGTS(
	segmentCount int,
	segmentDuration time.Duration,
	segmentMaxSize uint64,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackMPEG4Audio,
) *muxerVariantMPEGTS {
	v := &muxerVariantMPEGTS{}

	v.playlist = newMuxerVariantMPEGTSPlaylist(segmentCount)

	v.segmenter = newMuxerVariantMPEGTSSegmenter(
		segmentDuration,
		segmentMaxSize,
		videoTrack,
		audioTrack,
		func(seg *muxerVariantMPEGTSSegment) {
			v.playlist.pushSegment(seg)
		},
	)

	return v
}

func (v *muxerVariantMPEGTS) close() {
	v.playlist.close()
}

func (v *muxerVariantMPEGTS) writeH264(ntp time.Time, pts time.Duration, nalus [][]byte) error {
	return v.segmenter.writeH264(ntp, pts, nalus)
}

func (v *muxerVariantMPEGTS) writeAAC(ntp time.Time, pts time.Duration, au []byte) error {
	return v.segmenter.writeAAC(ntp, pts, au)
}

func (v *muxerVariantMPEGTS) file(name string, msn string, part string, skip string) *MuxerFileResponse {
	return v.playlist.file(name)
}
