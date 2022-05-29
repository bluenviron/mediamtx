package hls

import (
	"time"

	"github.com/aler9/gortsplib"
)

const (
	fmp4VideoTimescale = 90000
)

func estimateDTSError(dts time.Duration, pts time.Duration, videoSampleDefaultDuration time.Duration) time.Duration {
	v := pts

	lastDiff := v - dts
	if lastDiff < 0 {
		lastDiff = -lastDiff
	}

	for {
		sign := time.Duration(1)
		if (v - dts) > 0 {
			sign = -1
		}

		newV := v + sign*videoSampleDefaultDuration

		diff := newV - dts
		if diff < 0 {
			diff = -diff
		}

		if diff > lastDiff {
			break
		}

		v = newV
		lastDiff = diff
	}

	return v - dts
}

type fmp4VideoSample struct {
	pts        time.Duration
	dts        time.Duration
	avcc       []byte
	idrPresent bool
	next       *fmp4VideoSample
}

func (s *fmp4VideoSample) fillDTS(
	videoSampleDefaultDuration time.Duration,
	prevDTS time.Duration,
) {
	if s.idrPresent {
		s.dts = s.pts
	} else {
		s.dts = prevDTS + videoSampleDefaultDuration
		s.dts += estimateDTSError(s.dts, s.pts, videoSampleDefaultDuration)
	}
}

func (s fmp4VideoSample) duration() time.Duration {
	return s.next.dts - s.dts
}

type fmp4AudioSample struct {
	pts  time.Duration
	au   []byte
	next *fmp4AudioSample
}

func (s fmp4AudioSample) duration() time.Duration {
	return s.next.pts - s.pts
}

type muxerVariantFMP4 struct {
	playlist  *muxerVariantFMP4Playlist
	segmenter *muxerVariantFMP4Segmenter
}

func newMuxerVariantFMP4(
	lowLatency bool,
	segmentCount int,
	segmentDuration time.Duration,
	partDuration time.Duration,
	segmentMaxSize uint64,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
) *muxerVariantFMP4 {
	v := &muxerVariantFMP4{}

	v.playlist = newMuxerVariantFMP4Playlist(
		lowLatency,
		segmentCount,
		videoTrack,
		audioTrack,
	)

	v.segmenter = newMuxerVariantFMP4Segmenter(
		lowLatency,
		segmentCount,
		segmentDuration,
		partDuration,
		segmentMaxSize,
		videoTrack,
		audioTrack,
		v.playlist.onSegmentFinalized,
		v.playlist.onPartFinalized,
	)

	return v
}

func (v *muxerVariantFMP4) close() {
	v.playlist.close()
}

func (v *muxerVariantFMP4) writeH264(pts time.Duration, nalus [][]byte) error {
	return v.segmenter.writeH264(pts, nalus)
}

func (v *muxerVariantFMP4) writeAAC(pts time.Duration, aus [][]byte) error {
	return v.segmenter.writeAAC(pts, aus)
}

func (v *muxerVariantFMP4) file(name string, msn string, part string, skip string) *MuxerFileResponse {
	return v.playlist.file(name, msn, part, skip)
}
