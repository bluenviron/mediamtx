package hls

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/aler9/gortsplib"
)

type partsReader struct {
	parts   []*muxerVariantFMP4Part
	curPart int
	curPos  int
}

func (mbr *partsReader) Read(p []byte) (int, error) {
	n := 0
	lenp := len(p)

	for {
		if mbr.curPart >= len(mbr.parts) {
			return n, io.EOF
		}

		copied := copy(p[n:], mbr.parts[mbr.curPart].renderedContent[mbr.curPos:])
		mbr.curPos += copied
		n += copied

		if mbr.curPos == len(mbr.parts[mbr.curPart].renderedContent) {
			mbr.curPart++
			mbr.curPos = 0
		}

		if n == lenp {
			return n, nil
		}
	}
}

type muxerVariantFMP4Segment struct {
	lowLatency      bool
	id              uint64
	startTime       time.Time
	startDTS        time.Duration
	segmentMaxSize  uint64
	videoTrack      *gortsplib.TrackH264
	audioTrack      *gortsplib.TrackAAC
	genPartID       func() uint64
	onPartFinalized func(*muxerVariantFMP4Part)

	size             uint64
	parts            []*muxerVariantFMP4Part
	currentPart      *muxerVariantFMP4Part
	renderedDuration time.Duration
}

func newMuxerVariantFMP4Segment(
	lowLatency bool,
	id uint64,
	startTime time.Time,
	startDTS time.Duration,
	segmentMaxSize uint64,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
	genPartID func() uint64,
	onPartFinalized func(*muxerVariantFMP4Part),
) *muxerVariantFMP4Segment {
	s := &muxerVariantFMP4Segment{
		lowLatency:      lowLatency,
		id:              id,
		startTime:       startTime,
		startDTS:        startDTS,
		segmentMaxSize:  segmentMaxSize,
		videoTrack:      videoTrack,
		audioTrack:      audioTrack,
		genPartID:       genPartID,
		onPartFinalized: onPartFinalized,
	}

	s.currentPart = newMuxerVariantFMP4Part(
		s.videoTrack,
		s.audioTrack,
		s.genPartID(),
	)

	return s
}

func (s *muxerVariantFMP4Segment) name() string {
	return "seg" + strconv.FormatUint(s.id, 10)
}

func (s *muxerVariantFMP4Segment) reader() io.Reader {
	return &partsReader{parts: s.parts}
}

func (s *muxerVariantFMP4Segment) getRenderedDuration() time.Duration {
	return s.renderedDuration
}

func (s *muxerVariantFMP4Segment) finalize(
	nextVideoSample *fmp4VideoSample,
	nextAudioSample *fmp4AudioSample,
) error {
	err := s.currentPart.finalize()
	if err != nil {
		return err
	}

	if s.currentPart.renderedContent != nil {
		s.onPartFinalized(s.currentPart)
		s.parts = append(s.parts, s.currentPart)
	}

	s.currentPart = nil

	if s.videoTrack != nil {
		s.renderedDuration = nextVideoSample.dts - s.startDTS
	} else {
		s.renderedDuration = 0
		for _, pa := range s.parts {
			s.renderedDuration += pa.renderedDuration
		}
	}

	return nil
}

func (s *muxerVariantFMP4Segment) writeH264(sample *fmp4VideoSample, adjustedPartDuration time.Duration) error {
	size := uint64(len(sample.avcc))

	if (s.size + size) > s.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}

	s.currentPart.writeH264(sample)

	s.size += size

	// switch part
	if s.lowLatency &&
		s.currentPart.duration() >= adjustedPartDuration {
		err := s.currentPart.finalize()
		if err != nil {
			return err
		}

		s.parts = append(s.parts, s.currentPart)
		s.onPartFinalized(s.currentPart)

		s.currentPart = newMuxerVariantFMP4Part(
			s.videoTrack,
			s.audioTrack,
			s.genPartID(),
		)
	}

	return nil
}

func (s *muxerVariantFMP4Segment) writeAAC(sample *fmp4AudioSample, adjustedPartDuration time.Duration) error {
	size := uint64(len(sample.au))

	if (s.size + size) > s.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}

	s.currentPart.writeAAC(sample)

	s.size += size

	// switch part
	if s.lowLatency && s.videoTrack == nil &&
		s.currentPart.duration() >= adjustedPartDuration {
		err := s.currentPart.finalize()
		if err != nil {
			return err
		}

		s.parts = append(s.parts, s.currentPart)
		s.onPartFinalized(s.currentPart)

		s.currentPart = newMuxerVariantFMP4Part(
			s.videoTrack,
			s.audioTrack,
			s.genPartID(),
		)
	}

	return nil
}
