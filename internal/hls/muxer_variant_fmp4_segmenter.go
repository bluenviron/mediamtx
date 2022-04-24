package hls

import (
	"bytes"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
)

type muxerVariantFMP4Segmenter struct {
	segmentDuration time.Duration
	segmentMaxSize  uint64
	videoTrack      *gortsplib.TrackH264
	audioTrack      *gortsplib.TrackAAC
	onSegmentReady  func(*muxerVariantFMP4Segment)

	nextSequenceNumber int
	currentSegment     *muxerVariantFMP4Segment
	startTime          time.Time
	startPTS           time.Duration
	lastSPS            []byte
	lastPPS            []byte
}

func newMuxerVariantFMP4Segmenter(
	segmentDuration time.Duration,
	segmentMaxSize uint64,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
	onSegmentReady func(*muxerVariantFMP4Segment),
) *muxerVariantFMP4Segmenter {
	m := &muxerVariantFMP4Segmenter{
		segmentDuration:    segmentDuration,
		segmentMaxSize:     segmentMaxSize,
		videoTrack:         videoTrack,
		audioTrack:         audioTrack,
		onSegmentReady:     onSegmentReady,
		nextSequenceNumber: 1,
	}

	return m
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
		m.lastSPS = m.videoTrack.SPS()
		m.lastPPS = m.videoTrack.PPS()
		m.startTime = now
		m.currentSegment = newMuxerVariantFMP4Segment(
			m.nextSequenceNumber, now, m.segmentMaxSize, m.videoTrack, m.audioTrack)
		m.nextSequenceNumber++
		m.startPTS = pts
		pts = 0
	} else {
		pts -= m.startPTS

		// switch segment
		if idrPresent &&
			m.currentSegment.startPTS != nil {
			sps := m.videoTrack.SPS()
			pps := m.videoTrack.PPS()

			if (pts-*m.currentSegment.startPTS) >= m.segmentDuration ||
				!bytes.Equal(m.lastSPS, sps) ||
				!bytes.Equal(m.lastPPS, pps) {
				err := m.currentSegment.finalize(&pts, m.startTime)
				if err != nil {
					return err
				}

				m.lastSPS = sps
				m.lastPPS = pps
				m.onSegmentReady(m.currentSegment)
				m.currentSegment = newMuxerVariantFMP4Segment(
					m.nextSequenceNumber, now, m.segmentMaxSize, m.videoTrack, m.audioTrack)
				m.nextSequenceNumber++
			}
		}
	}

	err := m.currentSegment.writeH264(pts, idrPresent, nalus)
	if err != nil {
		err := m.currentSegment.finalize(nil, m.startTime)
		if err != nil {
			return err
		}

		m.onSegmentReady(m.currentSegment)
		m.currentSegment = nil
		return err
	}

	return nil
}

func (m *muxerVariantFMP4Segmenter) writeAAC(pts time.Duration, aus [][]byte) error {
	now := time.Now()

	if m.videoTrack == nil {
		if m.currentSegment == nil {
			// create first segment
			m.startTime = now
			m.currentSegment = newMuxerVariantFMP4Segment(
				m.nextSequenceNumber, now, m.segmentMaxSize, m.videoTrack, m.audioTrack)
			m.nextSequenceNumber++
			m.startPTS = pts
			pts = 0
		} else {
			pts -= m.startPTS

			// switch segment
			if m.currentSegment.audioAUCount >= segmentMinAUCount &&
				m.currentSegment.startPTS != nil &&
				(pts-*m.currentSegment.startPTS) >= m.segmentDuration {
				err := m.currentSegment.finalize(&pts, m.startTime)
				if err != nil {
					return err
				}

				m.onSegmentReady(m.currentSegment)
				m.currentSegment = newMuxerVariantFMP4Segment(
					m.nextSequenceNumber, now, m.segmentMaxSize, m.videoTrack, m.audioTrack)
				m.nextSequenceNumber++
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
		err := m.currentSegment.finalize(nil, m.startTime)
		if err != nil {
			return err
		}

		m.onSegmentReady(m.currentSegment)
		m.currentSegment = nil
		return err
	}

	return nil
}
