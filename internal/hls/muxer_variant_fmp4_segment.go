package hls

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
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
	partDuration    time.Duration
	segmentMaxSize  uint64
	videoTrack      *gortsplib.TrackH264
	audioTrack      *gortsplib.TrackAAC
	genPartID       func() uint64
	onPartFinalized func(*muxerVariantFMP4Part)

	videoSampleDuration time.Duration
	videoEntriesCount   int
	size                uint64
	parts               []*muxerVariantFMP4Part
	currentPart         *muxerVariantFMP4Part
	renderedDuration    time.Duration
}

func newMuxerVariantFMP4Segment(
	lowLatency bool,
	id uint64,
	startTime time.Time,
	startDTS time.Duration,
	partDuration time.Duration,
	segmentMaxSize uint64,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
	genPartID func() uint64,
	onPartFinalized func(*muxerVariantFMP4Part),
) (*muxerVariantFMP4Segment, error) {
	s := &muxerVariantFMP4Segment{
		lowLatency:      lowLatency,
		id:              id,
		startTime:       startTime,
		startDTS:        startDTS,
		partDuration:    partDuration,
		segmentMaxSize:  segmentMaxSize,
		videoTrack:      videoTrack,
		audioTrack:      audioTrack,
		genPartID:       genPartID,
		onPartFinalized: onPartFinalized,
	}

	if s.videoTrack != nil {
		var spsp h264.SPS
		err := spsp.Unmarshal(s.videoTrack.SPS())
		if err != nil {
			return nil, err
		}

		fps := spsp.FPS()
		if fps == 0 {
			return nil, fmt.Errorf("unable to obtain video FPS")
		}

		s.videoSampleDuration = time.Duration(float64(time.Second) / fps)
	}

	s.currentPart = newMuxerVariantFMP4Part(
		s.videoTrack,
		s.audioTrack,
		s.genPartID(),
		s.startDTS,
		s.videoSampleDuration,
	)

	return s, nil
}

func (s *muxerVariantFMP4Segment) name() string {
	return "seg" + strconv.FormatUint(s.id, 10)
}

func (s *muxerVariantFMP4Segment) reader() io.Reader {
	return &partsReader{parts: s.parts}
}

func (s *muxerVariantFMP4Segment) finalize(
	nextVideoEntry *fmp4VideoEntry,
	nextAudioEntry *fmp4AudioEntry,
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
		s.renderedDuration = nextVideoEntry.pts - s.startDTS
	} else {
		s.renderedDuration = nextAudioEntry.pts - s.startDTS
	}

	return nil
}

func (s *muxerVariantFMP4Segment) writeH264(entry *fmp4VideoEntry) error {
	size := uint64(len(entry.avcc))

	if (s.size + size) > s.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}

	s.currentPart.writeH264(entry)

	s.videoEntriesCount++
	s.size += size

	if s.lowLatency &&
		s.currentPart.duration() >= s.partDuration {
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
			s.startDTS+time.Duration(s.videoEntriesCount)*s.videoSampleDuration,
			s.videoSampleDuration,
		)
	}

	return nil
}

func (s *muxerVariantFMP4Segment) writeAAC(entry *fmp4AudioEntry) error {
	size := uint64(len(entry.au))

	if (s.size + size) > s.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}

	s.currentPart.writeAAC(entry)

	s.size += size

	if s.lowLatency && s.videoTrack == nil &&
		s.currentPart.duration() >= s.partDuration {
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
			entry.next.pts,
			s.videoSampleDuration,
		)
	}

	return nil
}
