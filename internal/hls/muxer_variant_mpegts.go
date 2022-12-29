package hls

import (
	"fmt"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"
)

type muxerVariantMPEGTS struct {
	playlist  *muxerVariantMPEGTSPlaylist
	segmenter *muxerVariantMPEGTSSegmenter
}

func newMuxerVariantMPEGTS(
	segmentCount int,
	segmentDuration time.Duration,
	segmentMaxSize uint64,
	videoTrack format.Format,
	audioTrack *format.MPEG4Audio,
) (*muxerVariantMPEGTS, error) {
	var videoTrackH264 *format.H264
	if videoTrack != nil {
		var ok bool
		videoTrackH264, ok = videoTrack.(*format.H264)
		if !ok {
			return nil, fmt.Errorf(
				"the MPEG-TS variant of HLS doesn't support H265. Use the fMP4 or Low-Latency variants instead")
		}
	}

	v := &muxerVariantMPEGTS{}

	v.playlist = newMuxerVariantMPEGTSPlaylist(segmentCount)

	v.segmenter = newMuxerVariantMPEGTSSegmenter(
		segmentDuration,
		segmentMaxSize,
		videoTrackH264,
		audioTrack,
		func(seg *muxerVariantMPEGTSSegment) {
			v.playlist.pushSegment(seg)
		},
	)

	return v, nil
}

func (v *muxerVariantMPEGTS) close() {
	v.playlist.close()
}

func (v *muxerVariantMPEGTS) writeH26x(ntp time.Time, pts time.Duration, nalus [][]byte) error {
	return v.segmenter.writeH264(ntp, pts, nalus)
}

func (v *muxerVariantMPEGTS) writeAAC(ntp time.Time, pts time.Duration, au []byte) error {
	return v.segmenter.writeAAC(ntp, pts, au)
}

func (v *muxerVariantMPEGTS) file(name string, msn string, part string, skip string) *MuxerFileResponse {
	return v.playlist.file(name)
}
