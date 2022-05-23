package hls

import (
	"bytes"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
)

type muxerVariantFMP4Segmenter struct {
	lowLatency         bool
	segmentDuration    time.Duration
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
}

func newMuxerVariantFMP4Segmenter(
	lowLatency bool,
	segmentDuration time.Duration,
	segmentMaxSize uint64,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
	onSegmentFinalized func(*muxerVariantFMP4Segment),
	onPartFinalized func(*muxerVariantFMP4Part),
) *muxerVariantFMP4Segmenter {
	m := &muxerVariantFMP4Segmenter{
		lowLatency:         lowLatency,
		segmentDuration:    segmentDuration,
		segmentMaxSize:     segmentMaxSize,
		videoTrack:         videoTrack,
		audioTrack:         audioTrack,
		onSegmentFinalized: onSegmentFinalized,
		onPartFinalized:    onPartFinalized,
	}

	return m
}

func (m *muxerVariantFMP4Segmenter) reset() {
	m.currentSegment = nil
	m.startPTS = 0
	m.lastSPS = nil
	m.lastPPS = nil
	m.nextSegmentID = 0
	m.nextPartID = 0
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
	now := time.Now()
	idrPresent := h264.IDRPresent(nalus)

	if m.currentSegment == nil {
		// skip groups silently until we find one with a IDR
		if !idrPresent {
			return nil
		}

		// create first segment
		var err error
		m.currentSegment, err = newMuxerVariantFMP4Segment(
			m.lowLatency,
			m.genSegmentID(),
			now,
			0,
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
		m.startPTS = pts
		pts = 0
	} else {
		pts -= m.startPTS

		// switch segment
		if idrPresent {
			sps := m.videoTrack.SPS()
			pps := m.videoTrack.PPS()

			if (pts-m.currentSegment.startDTS) >= m.segmentDuration ||
				!bytes.Equal(m.lastSPS, sps) ||
				!bytes.Equal(m.lastPPS, pps) {
				lastAudioEntry, err := m.currentSegment.finalize()
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
					pts,
					m.segmentMaxSize,
					m.videoTrack,
					m.audioTrack,
					m.genPartID,
					m.onPartFinalized,
				)
				if err != nil {
					return err
				}

				if lastAudioEntry != nil {
					m.currentSegment.writeAAC(lastAudioEntry.pts, [][]byte{lastAudioEntry.au})
				}
			}
		}
	}

	err := m.currentSegment.writeH264(pts, nalus)
	if err != nil {
		_, err := m.currentSegment.finalize()
		if err != nil {
			return err
		}
		m.onSegmentFinalized(m.currentSegment)

		m.reset()
		return err
	}

	return nil
}

func (m *muxerVariantFMP4Segmenter) writeAAC(pts time.Duration, aus [][]byte) error {
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
				m.segmentMaxSize,
				m.videoTrack,
				m.audioTrack,
				m.genPartID,
				m.onPartFinalized,
			)
			if err != nil {
				return err
			}

			m.startPTS = pts
			pts = 0
		} else {
			pts -= m.startPTS

			// switch segment
			if (pts-m.currentSegment.startDTS) >= m.segmentDuration &&
				m.currentSegment.audioEntriesCount >= segmentMinAUCount {
				lastAudioEntry, err := m.currentSegment.finalize()
				if err != nil {
					return err
				}
				m.onSegmentFinalized(m.currentSegment)

				m.currentSegment, err = newMuxerVariantFMP4Segment(
					m.lowLatency,
					m.genSegmentID(),
					now,
					pts,
					m.segmentMaxSize,
					m.videoTrack,
					m.audioTrack,
					m.genPartID,
					m.onPartFinalized,
				)
				if err != nil {
					return err
				}

				if lastAudioEntry != nil {
					m.currentSegment.writeAAC(lastAudioEntry.pts, [][]byte{lastAudioEntry.au})
				}
			}
		}
	} else {
		// wait for the video track
		if m.currentSegment == nil {
			return nil
		}

		pts -= m.startPTS
	}

	err := m.currentSegment.writeAAC(pts, aus)
	if err != nil {
		_, err := m.currentSegment.finalize()
		if err != nil {
			return err
		}
		m.onSegmentFinalized(m.currentSegment)

		m.reset()
		return err
	}

	return nil
}
