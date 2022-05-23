package hls

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/aac"
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

	sampleDuration    time.Duration
	videoEntriesCount int
	audioEntriesCount int
	entriesSize       uint64
	parts             []*muxerVariantFMP4Part
	currentPart       *muxerVariantFMP4Part
	duration          time.Duration
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

	if s.videoTrack != nil {
		var spsp h264.SPS
		err := spsp.Unmarshal(s.videoTrack.SPS())
		if err != nil {
			return nil, err
		}

		s.sampleDuration = time.Duration(float64(time.Second) / spsp.FPS())
	}

	s.currentPart = newMuxerVariantFMP4Part(
		s.videoTrack,
		s.audioTrack,
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

func (s *muxerVariantFMP4Segment) finalize() (*fmp4PartAudioEntry, error) {
	lastAudioEntry, err := s.currentPart.finalize()
	if err != nil {
		return nil, err
	}

	if lastAudioEntry != nil {
		s.audioEntriesCount--
	}

	s.onPartFinalized(s.currentPart)
	s.parts = append(s.parts, s.currentPart)
	s.currentPart = nil

	if s.videoTrack != nil {
		s.duration = time.Duration(s.videoEntriesCount) * s.sampleDuration
	} else {
		s.duration = time.Duration(s.audioEntriesCount) * aac.SamplesPerAccessUnit *
			time.Second / time.Duration(s.audioTrack.ClockRate())
	}

	return lastAudioEntry, nil
}

func (s *muxerVariantFMP4Segment) writeH264(
	pts time.Duration,
	nalus [][]byte,
) error {
	size := uint64(0)
	for _, nalu := range nalus {
		size += uint64(len(nalu))
	}

	if (s.entriesSize + size) > s.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}

	err := s.currentPart.writeH264(pts, nalus)
	if err != nil {
		return err
	}

	s.videoEntriesCount++
	s.entriesSize += size

	if s.lowLatency && len(s.currentPart.videoEntries) >= fmp4MinVideoEntriesPerPart &&
		(s.audioTrack == nil || len(s.currentPart.audioEntries) >= 2) {
		lastAudioEntry, err := s.currentPart.finalize()
		if err != nil {
			return err
		}

		s.parts = append(s.parts, s.currentPart)
		s.onPartFinalized(s.currentPart)

		s.currentPart = newMuxerVariantFMP4Part(
			s.videoTrack,
			s.audioTrack,
			s.genPartID(),
			s.startDTS+time.Duration(s.videoEntriesCount)*s.sampleDuration,
			s.sampleDuration,
		)

		if lastAudioEntry != nil {
			s.currentPart.writeAAC(lastAudioEntry.pts, [][]byte{lastAudioEntry.au})
		}
	}

	return nil
}

func (s *muxerVariantFMP4Segment) writeAAC(
	pts time.Duration,
	aus [][]byte,
) error {
	size := uint64(0)
	for _, au := range aus {
		size += uint64(len(au))
	}

	if (s.entriesSize + size) > s.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}

	err := s.currentPart.writeAAC(pts, aus)
	if err != nil {
		return err
	}

	s.audioEntriesCount += len(aus)
	s.entriesSize += size

	if s.lowLatency && s.videoTrack == nil &&
		len(s.currentPart.audioEntries) > fmp4MinAudioEntriesPerPart {
		lastAudioEntry, err := s.currentPart.finalize()
		if err != nil {
			return err
		}

		if lastAudioEntry != nil {
			s.audioEntriesCount--
		}

		s.parts = append(s.parts, s.currentPart)
		s.onPartFinalized(s.currentPart)

		s.currentPart = newMuxerVariantFMP4Part(
			s.videoTrack,
			s.audioTrack,
			s.genPartID(),
			s.startDTS+time.Duration(s.audioEntriesCount)*aac.SamplesPerAccessUnit*
				time.Second/time.Duration(s.audioTrack.ClockRate()),
			s.sampleDuration,
		)

		if lastAudioEntry != nil {
			s.currentPart.writeAAC(lastAudioEntry.pts, [][]byte{lastAudioEntry.au})
			s.audioEntriesCount++
		}
	}

	return nil
}
