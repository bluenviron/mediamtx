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

	startDTS              time.Duration
	videoFirstIDRReceived bool
	videoDTSExtractor     *h264.DTSExtractor
	videoSPS              []byte
	currentSegment        *muxerVariantFMP4Segment
	nextSegmentID         uint64
	nextPartID            uint64
	nextVideoSample       *fmp4VideoSample
	nextAudioSample       *fmp4AudioSample
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

// iPhone iOS fails if part durations are less than 85% of maximum part duration.
// find a part duration that is compatible with all received sample durations
func (m *muxerVariantFMP4Segmenter) adjustPartDuration(du time.Duration) {
	if !m.lowLatency || m.firstSegmentFinalized {
		return
	}

	if _, ok := m.sampleDurations[du]; !ok {
		m.sampleDurations[du] = struct{}{}
		m.adjustedPartDuration = findCompatiblePartDuration(
			m.partDuration,
			m.sampleDurations,
		)
	}
}

func (m *muxerVariantFMP4Segmenter) writeH264(pts time.Duration, nalus [][]byte) error {
	idrPresent := false
	nonIDRPresent := false

	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)
		switch typ {
		case h264.NALUTypeIDR:
			idrPresent = true

		case h264.NALUTypeNonIDR:
			nonIDRPresent = true
		}
	}

	if !idrPresent && !nonIDRPresent {
		return nil
	}

	avcc, err := h264.AVCCMarshal(nalus)
	if err != nil {
		return err
	}

	return m.writeH264Entry(&fmp4VideoSample{
		pts:        pts,
		nalus:      nalus,
		avcc:       avcc,
		idrPresent: idrPresent,
	})
}

func (m *muxerVariantFMP4Segmenter) writeH264Entry(sample *fmp4VideoSample) error {
	if !m.videoFirstIDRReceived {
		// skip sample silently until we find one with an IDR
		if !sample.idrPresent {
			return nil
		}

		m.videoFirstIDRReceived = true
		m.videoDTSExtractor = h264.NewDTSExtractor()
		m.videoSPS = append([]byte(nil), m.videoTrack.SafeSPS()...)

		var err error
		sample.dts, err = m.videoDTSExtractor.Extract(sample.nalus, sample.pts)
		if err != nil {
			return err
		}
		sample.nalus = nil

		m.startDTS = sample.dts
		sample.dts = 0
		sample.pts -= m.startDTS
	} else {
		var err error
		sample.dts, err = m.videoDTSExtractor.Extract(sample.nalus, sample.pts)
		if err != nil {
			return err
		}
		sample.nalus = nil

		sample.dts -= m.startDTS
		sample.pts -= m.startDTS
	}

	// put samples into a queue in order to
	// - allow to compute sample duration
	// - check if next sample is IDR
	sample, m.nextVideoSample = m.nextVideoSample, sample
	if sample == nil {
		return nil
	}
	sample.next = m.nextVideoSample

	now := time.Now()

	if m.currentSegment == nil {
		// create first segment
		m.currentSegment = newMuxerVariantFMP4Segment(
			m.lowLatency,
			m.genSegmentID(),
			now,
			sample.dts,
			m.segmentMaxSize,
			m.videoTrack,
			m.audioTrack,
			m.genPartID,
			m.onPartFinalized,
		)
	}

	m.adjustPartDuration(sample.duration())

	err := m.currentSegment.writeH264(sample, m.adjustedPartDuration)
	if err != nil {
		return err
	}

	// switch segment
	if sample.next.idrPresent {
		sps := m.videoTrack.SafeSPS()
		spsChanged := !bytes.Equal(m.videoSPS, sps)

		if (sample.next.dts-m.currentSegment.startDTS) >= m.segmentDuration ||
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
				sample.next.dts,
				m.segmentMaxSize,
				m.videoTrack,
				m.audioTrack,
				m.genPartID,
				m.onPartFinalized,
			)

			// if SPS changed, reset adjusted part duration
			if spsChanged {
				m.videoSPS = append([]byte(nil), sps...)
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
	if m.videoTrack != nil {
		// wait for the video track
		if !m.videoFirstIDRReceived {
			return nil
		}

		sample.pts -= m.startDTS
	}

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
				sample.pts,
				m.segmentMaxSize,
				m.videoTrack,
				m.audioTrack,
				m.genPartID,
				m.onPartFinalized,
			)
		}
	} else {
		// wait for the video track
		if m.currentSegment == nil {
			return nil
		}
	}

	err := m.currentSegment.writeAAC(sample, m.partDuration)
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
