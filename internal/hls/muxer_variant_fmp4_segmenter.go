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

	currentSegment  *muxerVariantFMP4Segment
	startPTS        time.Duration
	lastSPS         []byte
	lastPPS         []byte
	nextSegmentID   uint64
	nextPartID      uint64
	nextVideoSample *fmp4VideoSample
	nextAudioSample *fmp4AudioSample
}

func newMuxerVariantFMP4Segmenter(
	lowLatency bool,
	segmentCount int,
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
		nextSegmentID:      uint64(segmentCount),
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

	return m.writeH264Entry(&fmp4VideoSample{
		pts:        pts,
		avcc:       avcc,
		idrPresent: idrPresent,
	})
}

func (m *muxerVariantFMP4Segmenter) writeH264Entry(sample *fmp4VideoSample) error {
	sample.pts -= m.startPTS

	// put sample into a queue in order to
	// - allow to compute sample dts
	// - allow to compute sample duration
	// - check if next sample is IDR
	sample, m.nextVideoSample = m.nextVideoSample, sample
	if sample == nil {
		return nil
	}
	sample.next = m.nextVideoSample

	now := time.Now()

	if m.currentSegment == nil {
		// skip groups silently until we find one with a IDR
		if !sample.idrPresent {
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
		m.startPTS = sample.pts
		sample.pts = 0
		sample.next.pts -= m.startPTS
	}

	err := m.currentSegment.writeH264(sample)
	if err != nil {
		return err
	}

	// switch segment
	if sample.next.idrPresent {
		sps := m.videoTrack.SPS()
		pps := m.videoTrack.PPS()

		if (sample.next.pts-m.currentSegment.startDTS) >= m.segmentDuration ||
			!bytes.Equal(m.lastSPS, sps) ||
			!bytes.Equal(m.lastPPS, pps) {
			err := m.currentSegment.finalize(sample.next, nil)
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
				sample.next.pts,
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
		err := m.writeAACEntry(&fmp4AudioSample{
			pts: pts + time.Duration(i)*aac.SamplesPerAccessUnit*time.Second/time.Duration(m.audioTrack.ClockRate()),
			au:  au,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *muxerVariantFMP4Segmenter) writeAACEntry(sample *fmp4AudioSample) error {
	sample.pts -= m.startPTS

	// put sample into a queue in order to
	// allow to compute the sample duration
	sample, m.nextAudioSample = m.nextAudioSample, sample
	if sample == nil {
		return nil
	}
	sample.next = m.nextAudioSample

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

			m.startPTS = sample.pts
			sample.pts = 0
			sample.next.pts -= m.startPTS
		}
	} else {
		// wait for the video track
		if m.currentSegment == nil {
			return nil
		}
	}

	err := m.currentSegment.writeAAC(sample)
	if err != nil {
		return err
	}

	// switch segment
	if m.videoTrack == nil &&
		(sample.next.pts-m.currentSegment.startDTS) >= m.segmentDuration {
		err := m.currentSegment.finalize(nil, sample.next)
		if err != nil {
			return err
		}
		m.onSegmentFinalized(m.currentSegment)

		m.currentSegment, err = newMuxerVariantFMP4Segment(
			m.lowLatency,
			m.genSegmentID(),
			now,
			sample.next.pts,
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
