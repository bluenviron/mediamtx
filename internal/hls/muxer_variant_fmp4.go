package hls

import (
	"time"

	"github.com/aler9/gortsplib"
)

const (
	fmp4VideoTimescale = 90000
)

type fmp4VideoSample struct {
	nalus      [][]byte
	pts        time.Duration
	dts        time.Duration
	avcc       []byte
	idrPresent bool
	next       *fmp4VideoSample
}

func (s fmp4VideoSample) duration() time.Duration {
	return s.next.dts - s.dts
}

type fmp4AudioSample struct {
	au   []byte
	pts  time.Duration
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
