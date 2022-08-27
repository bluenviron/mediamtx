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
	nextVideoSample       *fmp4.VideoSample
	nextAudioSample       *fmp4.AudioSample
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

	return m.writeH264Entry(now, &fmp4.VideoSample{
		PTS:        pts,
		NALUs:      nalus,
		IDRPresent: idrPresent,
	})
}

func (m *muxerVariantFMP4Segmenter) writeH264Entry(now time.Time, sample *fmp4.VideoSample) error {
	if !m.videoFirstIDRReceived {
		// skip sample silently until we find one with an IDR
		if !sample.IDRPresent {
			return nil
		}

		m.videoFirstIDRReceived = true
		m.videoDTSExtractor = h264.NewDTSExtractor()
		m.videoSPS = m.videoTrack.SafeSPS()

		var err error
		sample.DTS, err = m.videoDTSExtractor.Extract(sample.NALUs, sample.PTS)
		if err != nil {
			return err
		}
		sample.NALUs = nil

		m.startDTS = sample.DTS
		sample.DTS = 0
		sample.PTS -= m.startDTS
	} else {
		var err error
		sample.DTS, err = m.videoDTSExtractor.Extract(sample.NALUs, sample.PTS)
		if err != nil {
			return err
		}
		sample.NALUs = nil

		sample.DTS -= m.startDTS
		sample.PTS -= m.startDTS
	}

	// put samples into a queue in order to
	// - allow to compute sample duration
	// - check if next sample is IDR
	sample, m.nextVideoSample = m.nextVideoSample, sample
	if sample == nil {
		return nil
	}
	sample.Next = m.nextVideoSample

	if m.currentSegment == nil {
		// create first segment
		m.currentSegment = newMuxerVariantFMP4Segment(
			m.lowLatency,
			m.genSegmentID(),
			now,
			sample.DTS,
			m.segmentMaxSize,
			m.videoTrack,
			m.audioTrack,
			m.genPartID,
			m.onPartFinalized,
		)
	}

	m.adjustPartDuration(sample.Duration())

	err := m.currentSegment.writeH264(sample, m.adjustedPartDuration)
	if err != nil {
		return err
	}

	// switch segment
	if sample.Next.IDRPresent {
		sps := m.videoTrack.SafeSPS()
		spsChanged := !bytes.Equal(m.videoSPS, sps)

		if (sample.Next.DTS-m.currentSegment.startDTS) >= m.segmentDuration ||
			spsChanged {
			err := m.currentSegment.finalize(sample.Next, nil)
			if err != nil {
				return err
			}
			m.onSegmentFinalized(m.currentSegment)

			m.firstSegmentFinalized = true

			m.currentSegment = newMuxerVariantFMP4Segment(
				m.lowLatency,
				m.genSegmentID(),
				now,
				sample.Next.DTS,
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

func (m *muxerVariantFMP4Segmenter) writeAAC(now time.Time, pts time.Duration, au []byte) error {
	return m.writeAACEntry(now, &fmp4.AudioSample{
		PTS: pts,
		AU:  au,
	})
}

func (m *muxerVariantFMP4Segmenter) writeAACEntry(now time.Time, sample *fmp4.AudioSample) error {
	if m.videoTrack != nil {
		// wait for the video track
		if !m.videoFirstIDRReceived {
			return nil
		}

		sample.PTS -= m.startDTS
	}

	// put samples into a queue in order to
	// allow to compute the sample duration
	sample, m.nextAudioSample = m.nextAudioSample, sample
	if sample == nil {
		return nil
	}
	sample.Next = m.nextAudioSample

	if m.videoTrack == nil {
		if m.currentSegment == nil {
			// create first segment
			m.currentSegment = newMuxerVariantFMP4Segment(
				m.lowLatency,
				m.genSegmentID(),
				now,
				sample.PTS,
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
		(sample.Next.PTS-m.currentSegment.startDTS) >= m.segmentDuration {
		err := m.currentSegment.finalize(nil, sample.Next)
		if err != nil {
			return err
		}
		m.onSegmentFinalized(m.currentSegment)

		m.firstSegmentFinalized = true

		m.currentSegment = newMuxerVariantFMP4Segment(
			m.lowLatency,
			m.genSegmentID(),
			now,
			sample.Next.PTS,
			m.segmentMaxSize,
			m.videoTrack,
			m.audioTrack,
			m.genPartID,
			m.onPartFinalized,
		)
	}

	return nil
}
