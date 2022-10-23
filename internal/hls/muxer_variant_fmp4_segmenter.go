package hls

import (
	"bytes"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"

	"github.com/aler9/rtsp-simple-server/internal/hls/fmp4"
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

type augmentedVideoSample struct {
	fmp4.PartSample
	dts time.Duration
}

type augmentedAudioSample struct {
	fmp4.PartSample
	dts time.Duration
}

type muxerVariantFMP4Segmenter struct {
	lowLatency         bool
	segmentDuration    time.Duration
	partDuration       time.Duration
	segmentMaxSize     uint64
	videoTrack         *gortsplib.TrackH264
	audioTrack         *gortsplib.TrackMPEG4Audio
	onSegmentFinalized func(*muxerVariantFMP4Segment)
	onPartFinalized    func(*muxerVariantFMP4Part)

	startDTS              time.Duration
	videoFirstIDRReceived bool
	videoDTSExtractor     *h264.DTSExtractor
	videoSPS              []byte
	currentSegment        *muxerVariantFMP4Segment
	nextSegmentID         uint64
	nextPartID            uint64
	nextVideoSample       *augmentedVideoSample
	nextAudioSample       *augmentedAudioSample
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
	audioTrack *gortsplib.TrackMPEG4Audio,
	onSegmentFinalized func(*muxerVariantFMP4Segment),
	onPartFinalized func(*muxerVariantFMP4Part),
) *muxerVariantFMP4Segmenter {
	m := &muxerVariantFMP4Segmenter{
		lowLatency:         lowLatency,
		segmentDuration:    segmentDuration,
		partDuration:       partDuration,
		segmentMaxSize:     segmentMaxSize,
		videoTrack:         videoTrack,
		audioTrack:         audioTrack,
		onSegmentFinalized: onSegmentFinalized,
		onPartFinalized:    onPartFinalized,
		sampleDurations:    make(map[time.Duration]struct{}),
	}

	// add initial gaps, required by iOS LL-HLS
	if m.lowLatency {
		m.nextSegmentID = 7
	}

	return m
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

func (m *muxerVariantFMP4Segmenter) writeH264(now time.Time, pts time.Duration, nalus [][]byte) error {
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

	return m.writeH264Entry(now, pts, nalus, idrPresent)
}

func (m *muxerVariantFMP4Segmenter) writeH264Entry(
	now time.Time,
	pts time.Duration,
	nalus [][]byte,
	idrPresent bool,
) error {
	var dts time.Duration

	if !m.videoFirstIDRReceived {
		// skip sample silently until we find one with an IDR
		if !idrPresent {
			return nil
		}

		m.videoFirstIDRReceived = true
		m.videoDTSExtractor = h264.NewDTSExtractor()
		m.videoSPS = m.videoTrack.SafeSPS()

		var err error
		dts, err = m.videoDTSExtractor.Extract(nalus, pts)
		if err != nil {
			return err
		}

		m.startDTS = dts
		dts = 0
		pts -= m.startDTS
	} else {
		var err error
		dts, err = m.videoDTSExtractor.Extract(nalus, pts)
		if err != nil {
			return err
		}

		dts -= m.startDTS
		pts -= m.startDTS
	}

	avcc, err := h264.AVCCMarshal(nalus)
	if err != nil {
		return err
	}

	var flags uint32
	if !idrPresent {
		flags |= 1 << 16
	}

	sample := &augmentedVideoSample{
		PartSample: fmp4.PartSample{
			PTSOffset: int32(durationGoToMp4(pts-dts, 90000)),
			Flags:     flags,
			Payload:   avcc,
		},
		dts: dts,
	}

	// put samples into a queue in order to
	// - allow to compute sample duration
	// - check if next sample is IDR
	sample, m.nextVideoSample = m.nextVideoSample, sample
	if sample == nil {
		return nil
	}
	sample.Duration = uint32(durationGoToMp4(m.nextVideoSample.dts-sample.dts, 90000))

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

	m.adjustPartDuration(durationMp4ToGo(uint64(sample.Duration), 90000))

	err = m.currentSegment.writeH264(sample, m.adjustedPartDuration)
	if err != nil {
		return err
	}

	// switch segment
	if idrPresent {
		sps := m.videoTrack.SafeSPS()
		spsChanged := !bytes.Equal(m.videoSPS, sps)

		if (m.nextVideoSample.dts-m.currentSegment.startDTS) >= m.segmentDuration ||
			spsChanged {
			err := m.currentSegment.finalize(m.nextVideoSample.dts)
			if err != nil {
				return err
			}
			m.onSegmentFinalized(m.currentSegment)

			m.firstSegmentFinalized = true

			m.currentSegment = newMuxerVariantFMP4Segment(
				m.lowLatency,
				m.genSegmentID(),
				now,
				m.nextVideoSample.dts,
				m.segmentMaxSize,
				m.videoTrack,
				m.audioTrack,
				m.genPartID,
				m.onPartFinalized,
			)

			// if SPS changed, reset adjusted part duration
			if spsChanged {
				m.videoSPS = sps
				m.firstSegmentFinalized = false
				m.sampleDurations = make(map[time.Duration]struct{})
			}
		}
	}

	return nil
}

func (m *muxerVariantFMP4Segmenter) writeAAC(now time.Time, dts time.Duration, au []byte) error {
	if m.videoTrack != nil {
		// wait for the video track
		if !m.videoFirstIDRReceived {
			return nil
		}

		dts -= m.startDTS
	}

	sample := &augmentedAudioSample{
		PartSample: fmp4.PartSample{
			Payload: au,
		},
		dts: dts,
	}

	// put samples into a queue in order to
	// allow to compute the sample duration
	sample, m.nextAudioSample = m.nextAudioSample, sample
	if sample == nil {
		return nil
	}
	sample.Duration = uint32(durationGoToMp4(m.nextAudioSample.dts-sample.dts, uint32(m.audioTrack.ClockRate())))

	if m.videoTrack == nil {
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
		(m.nextAudioSample.dts-m.currentSegment.startDTS) >= m.segmentDuration {
		err := m.currentSegment.finalize(0)
		if err != nil {
			return err
		}
		m.onSegmentFinalized(m.currentSegment)

		m.firstSegmentFinalized = true

		m.currentSegment = newMuxerVariantFMP4Segment(
			m.lowLatency,
			m.genSegmentID(),
			now,
			m.nextAudioSample.dts,
			m.segmentMaxSize,
			m.videoTrack,
			m.audioTrack,
			m.genPartID,
			m.onPartFinalized,
		)
	}

	return nil
}
