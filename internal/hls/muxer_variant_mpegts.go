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
	audioTrack format.Format,
) (*muxerVariantMPEGTS, error) {
	var videoTrackH264 *format.H264
	if videoTrack != nil {
		var ok bool
		videoTrackH264, ok = videoTrack.(*format.H264)
		if !ok {
			return nil, fmt.Errorf(
				"the MPEG-TS variant of HLS only supports H264 video. Use the fMP4 or Low-Latency variants instead")
		}
	}

	var audioTrackMPEG4Audio *format.MPEG4Audio
	if audioTrack != nil {
		var ok bool
		audioTrackMPEG4Audio, ok = audioTrack.(*format.MPEG4Audio)
		if !ok {
			return nil, fmt.Errorf(
				"the MPEG-TS variant of HLS only supports MPEG4-audio. Use the fMP4 or Low-Latency variants instead")
		}
	}

	v := &muxerVariantMPEGTS{}

	v.playlist = newMuxerVariantMPEGTSPlaylist(segmentCount)

	v.segmenter = newMuxerVariantMPEGTSSegmenter(
		segmentDuration,
		segmentMaxSize,
		videoTrackH264,
		audioTrackMPEG4Audio,
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

func (v *muxerVariantMPEGTS) writeAudio(ntp time.Time, pts time.Duration, au []byte) error {
	return v.segmenter.writeAAC(ntp, pts, au)
}

func (v *muxerVariantMPEGTS) file(name string, msn string, part string, skip string) *MuxerFileResponse {
	return v.playlist.file(name)
}
