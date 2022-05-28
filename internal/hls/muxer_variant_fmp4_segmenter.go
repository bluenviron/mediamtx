package hls

import (
	"bytes"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/aac"
	"github.com/aler9/gortsplib/pkg/h264"
)

type muxerVariantFMP4Segmenter struct {
	lowLatency         bool
	segmentDuration    time.Duration
	partDuration       time.Duration
	segmentMaxSize     uint64
	videoTrack         *gortsplib.TrackH264
	audioTrack         *gortsplib.TrackAAC
	onSegmentFinalized func(*muxerVariantFMP4Segment)
	onPartFinalized    func(*muxerVariantFMP4Part)

	currentSegment *muxerVariantFMP4Segment
	startPTS       time.Duration
	lastSPS        []byte
	lastPPS        []byte
	nextSegmentID  uint64
	nextPartID     uint64
	nextVideoEntry *fmp4VideoEntry
	nextAudioEntry *fmp4AudioEntry
}

func newMuxerVariantFMP4Segmenter(
	lowLatency bool,
	segmentDuration time.Duration,
	partDuration time.Duration,
	segmentMaxSize uint64,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
	onSegmentFinalized func(*muxerVariantFMP4Segment),
	onPartFinalized func(*muxerVariantFMP4Part),
) *muxerVariantFMP4Segmenter {
	return &muxerVariantFMP4Segmenter{
		lowLatency:         lowLatency,
		segmentDuration:    segmentDuration,
		partDuration:       partDuration,
		segmentMaxSize:     segmentMaxSize,
		videoTrack:         videoTrack,
		audioTrack:         audioTrack,
		onSegmentFinalized: onSegmentFinalized,
		onPartFinalized:    onPartFinalized,
	}
}

func (m *muxerVariantFMP4Segmenter) genSegmentID() uint64 {
	id := m.nextSegmentID
	m.nextSegmentID++
	return id
}

func (m *muxerVariantFMP4Segmenter) genPartID() uint64 {
	id := m.nextPartID
	m.nextPartID++
	return id
}

func (m *muxerVariantFMP4Segmenter) writeH264(pts time.Duration, nalus [][]byte) error {
	avcc, err := h264.AVCCEncode(nalus)
	if err != nil {
		return err
	}

	idrPresent := h264.IDRPresent(nalus)

	return m.writeH264Entry(&fmp4VideoEntry{
		pts:        pts,
		avcc:       avcc,
		idrPresent: idrPresent,
	})
}

func (m *muxerVariantFMP4Segmenter) writeH264Entry(entry *fmp4VideoEntry) error {
	entry.pts -= m.startPTS

	// put one entry into a queue in order to
	// - allow duration computation
	// - check if next entry is IDR
	entry, m.nextVideoEntry = m.nextVideoEntry, entry
	if entry == nil {
		return nil
	}
	entry.next = m.nextVideoEntry

	now := time.Now()

	if m.currentSegment == nil {
		// skip groups silently until we find one with a IDR
		if !entry.idrPresent {
			return nil
		}

		// create first segment
		var err error
		m.currentSegment, err = newMuxerVariantFMP4Segment(
			m.lowLatency,
			m.genSegmentID(),
			now,
			0,
			m.partDuration,
			m.segmentMaxSize,
			m.videoTrack,
			m.audioTrack,
			m.genPartID,
			m.onPartFinalized,
		)
		if err != nil {
			return err
		}

		m.lastSPS = m.videoTrack.SPS()
		m.lastPPS = m.videoTrack.PPS()
		m.startPTS = entry.pts
		entry.pts = 0
		entry.next.pts -= m.startPTS
	}

	err := m.currentSegment.writeH264(entry)
	if err != nil {
		return err
	}

	// switch segment
	if entry.next.idrPresent {
		sps := m.videoTrack.SPS()
		pps := m.videoTrack.PPS()

		if (entry.next.pts-m.currentSegment.startDTS) >= m.segmentDuration ||
			!bytes.Equal(m.lastSPS, sps) ||
			!bytes.Equal(m.lastPPS, pps) {
			err := m.currentSegment.finalize(entry.next, nil)
			if err != nil {
				return err
			}
			m.onSegmentFinalized(m.currentSegment)

			m.lastSPS = sps
			m.lastPPS = pps
			m.currentSegment, err = newMuxerVariantFMP4Segment(
				m.lowLatency,
				m.genSegmentID(),
				now,
				entry.next.pts,
				m.partDuration,
				m.segmentMaxSize,
				m.videoTrack,
				m.audioTrack,
				m.genPartID,
				m.onPartFinalized,
			)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *muxerVariantFMP4Segmenter) writeAAC(pts time.Duration, aus [][]byte) error {
	for i, au := range aus {
		err := m.writeAACEntry(&fmp4AudioEntry{
			pts: pts + time.Duration(i)*aac.SamplesPerAccessUnit*time.Second/time.Duration(m.audioTrack.ClockRate()),
			au:  au,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *muxerVariantFMP4Segmenter) writeAACEntry(entry *fmp4AudioEntry) error {
	entry.pts -= m.startPTS

	// put one entry into a queue in order to allow to compute the duration of each entry.
	entry, m.nextAudioEntry = m.nextAudioEntry, entry
	if entry == nil {
		return nil
	}
	entry.next = m.nextAudioEntry

	now := time.Now()

	if m.videoTrack == nil {
		if m.currentSegment == nil {
			// create first segment
			var err error
			m.currentSegment, err = newMuxerVariantFMP4Segment(
				m.lowLatency,
				m.genSegmentID(),
				now,
				0,
				m.partDuration,
				m.segmentMaxSize,
				m.videoTrack,
				m.audioTrack,
				m.genPartID,
				m.onPartFinalized,
			)
			if err != nil {
				return err
			}

			m.startPTS = entry.pts
			entry.pts = 0
			entry.next.pts -= m.startPTS
		}
	} else {
		// wait for the video track
		if m.currentSegment == nil {
			return nil
		}
	}

	err := m.currentSegment.writeAAC(entry)
	if err != nil {
		return err
	}

	// switch segment
	if m.videoTrack == nil &&
		(entry.next.pts-m.currentSegment.startDTS) >= m.segmentDuration {
		err := m.currentSegment.finalize(nil, entry.next)
		if err != nil {
			return err
		}
		m.onSegmentFinalized(m.currentSegment)

		m.currentSegment, err = newMuxerVariantFMP4Segment(
			m.lowLatency,
			m.genSegmentID(),
			now,
			entry.next.pts,
			m.partDuration,
			m.segmentMaxSize,
			m.videoTrack,
			m.audioTrack,
			m.genPartID,
			m.onPartFinalized,
		)
		if err != nil {
			return err
		}
	}

	return nil
}
