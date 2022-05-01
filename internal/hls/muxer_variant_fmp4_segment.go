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

		copied := copy(p[n:], mbr.parts[mbr.curPart].rendered[mbr.curPos:])
		mbr.curPos += copied
		n += copied

		if mbr.curPos == len(mbr.parts[mbr.curPart].rendered) {
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

	sampleDuration time.Duration
	entriesCount   int
	entriesSize    uint64
	audioAUCount   int
	parts          []*muxerVariantFMP4Part
	currentPart    *muxerVariantFMP4Part
	duration       time.Duration
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
) (*muxerVariantFMP4Segment, error) {
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

	var spsp h264.SPS
	err := spsp.Unmarshal(s.videoTrack.SPS())
	if err != nil {
		return nil, err
	}

	s.sampleDuration = time.Duration(float64(time.Second) / spsp.FPS())
	s.currentPart = newMuxerVariantFMP4Part(
		s.genPartID(),
		s.startDTS,
		s.sampleDuration,
	)

	return s, nil
}

func (s *muxerVariantFMP4Segment) name() string {
	return "seg" + strconv.FormatUint(s.id, 10)
}

func (s *muxerVariantFMP4Segment) reader() io.Reader {
	return &partsReader{parts: s.parts}
}

func (s *muxerVariantFMP4Segment) finalize() error {
	err := s.currentPart.finalize()
	if err != nil {
		return err
	}

	s.onPartFinalized(s.currentPart)
	s.parts = append(s.parts, s.currentPart)
	s.currentPart = nil

	s.duration = time.Duration(s.entriesCount) * s.sampleDuration

	return nil
}

func (s *muxerVariantFMP4Segment) writeH264(
	pts time.Duration,
	nalus [][]byte,
) error {
	avcc, err := h264.AVCCEncode(nalus)
	if err != nil {
		return err
	}
	avccl := uint64(len(avcc))

	if (s.entriesSize + avccl) > s.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}

	err = s.currentPart.writeH264(pts, avcc)
	if err != nil {
		return err
	}

	s.entriesCount++
	s.entriesSize += avccl

	if s.lowLatency && len(s.currentPart.entries) > fmp4LowLatencyPartEntryCount {
		err := s.currentPart.finalize()
		if err != nil {
			return err
		}

		s.parts = append(s.parts, s.currentPart)
		s.onPartFinalized(s.currentPart)

		s.currentPart = newMuxerVariantFMP4Part(
			s.genPartID(),
			s.startDTS+time.Duration(s.entriesCount)*s.sampleDuration,
			s.sampleDuration,
		)
	}

	return nil
}

func (s *muxerVariantFMP4Segment) writeAAC(
	pts time.Duration,
	aus [][]byte,
) error {
	return nil
}
