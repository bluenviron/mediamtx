package hls

import (
	"bytes"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/aac"
	"github.com/aler9/gortsplib/pkg/h264"
)

func partDurationIsCompatible(partDuration time.Duration, sampleDuration time.Duration) bool {
	if sampleDuration > partDuration {
		return false
	}

	f := (partDuration / sampleDuration)
	if (partDuration % sampleDuration) != 0 {
		f++
	}
	f *= sampleDuration

	return partDuration > ((f * 85) / 100)
}

func findCompatiblePartDuration(
	minPartDuration time.Duration,
	sampleDurations map[time.Duration]struct{},
) time.Duration {
	i := minPartDuration
	for ; i < 5*time.Second; i += 5 * time.Millisecond {
		isCompatible := func() bool {
			for sd := range sampleDurations {
				if !partDurationIsCompatible(i, sd) {
					return false
				}
			}
			return true
		}()
		if isCompatible {
			break
		}
	}
	return i
}

type muxerVariantFMP4Segmenter struct {
	lowLatency         bool
	segmentDuration    time.Duration
	partDuration       time.Duration
	segmentMaxSize     uint64
	videoTrack         *gortsplib.TrackH264
	audioTrack         *gortsplib.TrackAAC
	onSegmentFinalized func(*muxerVariantFMP4Segment)
	onPartFinalized    func(*muxerVariantFMP4Part)

	currentSegment        *muxerVariantFMP4Segment
	startPTS              time.Duration
	videoSPSP             *h264.SPS
	videoSPS              []byte
	videoPPS              []byte
	videoNextSPSP         *h264.SPS
	videoNextSPS          []byte
	videoNextPPS          []byte
	nextSegmentID         uint64
	nextPartID            uint64
	nextVideoSample       *fmp4VideoSample
	nextAudioSample       *fmp4AudioSample
	videoExpectedPOC      uint32
	firstSegmentFinalized bool
	sampleDurations       map[time.Duration]struct{}
	adjustedPartDuration  time.Duration
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
		sampleDurations:    make(map[time.Duration]struct{}),
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

func (m *muxerVariantFMP4Segmenter) adjustPartDuration(du time.Duration) {
	if !m.lowLatency {
		return
	}
	if m.firstSegmentFinalized {
		return
	}

	// iPhone iOS fails if part durations are less than 85% of maximum part duration.
	// find a part duration that is compatible with all received sample durations
	if _, ok := m.sampleDurations[du]; !ok {
		m.sampleDurations[du] = struct{}{}
		m.adjustedPartDuration = findCompatiblePartDuration(
			m.partDuration,
			m.sampleDurations,
		)
	}
}

func (m *muxerVariantFMP4Segmenter) writeH264(pts time.Duration, nalus [][]byte) error {
	avcc, err := h264.AVCCEncode(nalus)
	if err != nil {
		return err
	}

	idrPresent := h264.IDRPresent(nalus)

	return m.writeH264Entry(&fmp4VideoSample{
		pts:        pts,
		nalus:      nalus,
		avcc:       avcc,
		idrPresent: idrPresent,
	})
}

func (m *muxerVariantFMP4Segmenter) writeH264Entry(sample *fmp4VideoSample) error {
	// put SPS/PPS into a queue in order to sync them with the sample queue
	m.videoSPSP = m.videoNextSPSP
	m.videoSPS = m.videoNextSPS
	m.videoPPS = m.videoNextPPS
	spsChanged := false
	if sample.idrPresent {
		videoNextSPS := m.videoTrack.SPS()
		videoNextPPS := m.videoTrack.PPS()

		if m.videoSPS == nil ||
			!bytes.Equal(m.videoNextSPS, videoNextSPS) ||
			!bytes.Equal(m.videoNextPPS, videoNextPPS) {
			spsChanged = true

			var videoSPSP h264.SPS
			err := videoSPSP.Unmarshal(videoNextSPS)
			if err != nil {
				return err
			}

			m.videoNextSPSP = &videoSPSP
			m.videoNextSPS = videoNextSPS
			m.videoNextPPS = videoNextPPS
		}
	}

	sample.pts -= m.startPTS

	// put samples into a queue in order to
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
		m.currentSegment = newMuxerVariantFMP4Segment(
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

		m.startPTS = sample.pts
		sample.pts = 0
		sample.next.pts -= m.startPTS
	}

	err := sample.next.fillDTS(sample, m.videoNextSPSP, &m.videoExpectedPOC)
	if err != nil {
		return err
	}
	sample.next.nalus = nil

	m.adjustPartDuration(sample.duration())

	err = m.currentSegment.writeH264(sample, m.adjustedPartDuration)
	if err != nil {
		return err
	}

	// switch segment
	if sample.next.idrPresent {
		if (sample.next.pts-m.currentSegment.startDTS) >= m.segmentDuration ||
			spsChanged {
			err := m.currentSegment.finalize(sample.next, nil)
			if err != nil {
				return err
			}
			m.onSegmentFinalized(m.currentSegment)

			m.firstSegmentFinalized = true

			m.currentSegment = newMuxerVariantFMP4Segment(
				m.lowLatency,
				m.genSegmentID(),
				now,
				sample.next.pts,
				m.segmentMaxSize,
				m.videoTrack,
				m.audioTrack,
				m.genPartID,
				m.onPartFinalized,
			)

			// if SPS changed, reset adjusted part duration
			if spsChanged {
				m.firstSegmentFinalized = false
				m.sampleDurations = make(map[time.Duration]struct{})
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

	// put samples into a queue in order to
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
			m.currentSegment = newMuxerVariantFMP4Segment(
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

	m.adjustPartDuration(sample.duration())

	err := m.currentSegment.writeAAC(sample, m.adjustedPartDuration)
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

		m.firstSegmentFinalized = true

		m.currentSegment = newMuxerVariantFMP4Segment(
			m.lowLatency,
			m.genSegmentID(),
			now,
			sample.next.pts,
			m.segmentMaxSize,
			m.videoTrack,
			m.audioTrack,
			m.genPartID,
			m.onPartFinalized,
		)
	}

	return nil
}
