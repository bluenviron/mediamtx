package hls

import (
	"fmt"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/codecs/h264"
	"github.com/aler9/gortsplib/v2/pkg/codecs/h265"
	"github.com/aler9/gortsplib/v2/pkg/format"

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

func partDurationIsCompatibleWithAll(partDuration time.Duration, sampleDurations map[time.Duration]struct{}) bool {
	for sd := range sampleDurations {
		if !partDurationIsCompatible(partDuration, sd) {
			return false
		}
	}
	return true
}

func findCompatiblePartDuration(
	minPartDuration time.Duration,
	sampleDurations map[time.Duration]struct{},
) time.Duration {
	i := minPartDuration
	for ; i < 5*time.Second; i += 5 * time.Millisecond {
		if partDurationIsCompatibleWithAll(i, sampleDurations) {
			break
		}
	}
	return i
}

type dtsExtractor interface {
	Extract([][]byte, time.Duration) (time.Duration, error)
}

func allocateDTSExtractor(track format.Format) dtsExtractor {
	switch track.(type) {
	case *format.H264:
		return h264.NewDTSExtractor()

	case *format.H265:
		return h265.NewDTSExtractor()
	}
	return nil
}

type augmentedVideoSample struct {
	fmp4.PartSample
	dts time.Duration
	ntp time.Time
}

type augmentedAudioSample struct {
	fmp4.PartSample
	dts time.Duration
	ntp time.Time
}

type muxerVariantFMP4Segmenter struct {
	lowLatency         bool
	segmentDuration    time.Duration
	partDuration       time.Duration
	segmentMaxSize     uint64
	videoTrack         format.Format
	audioTrack         format.Format
	onSegmentFinalized func(*muxerVariantFMP4Segment)
	onPartFinalized    func(*muxerVariantFMP4Part)

	startDTS                       time.Duration
	videoFirstRandomAccessReceived bool
	videoDTSExtractor              dtsExtractor
	lastVideoParams                [][]byte
	currentSegment                 *muxerVariantFMP4Segment
	nextSegmentID                  uint64
	nextPartID                     uint64
	nextVideoSample                *augmentedVideoSample
	nextAudioSample                *augmentedAudioSample
	firstSegmentFinalized          bool
	sampleDurations                map[time.Duration]struct{}
	adjustedPartDuration           time.Duration
}

func newMuxerVariantFMP4Segmenter(
	lowLatency bool,
	segmentCount int,
	segmentDuration time.Duration,
	partDuration time.Duration,
	segmentMaxSize uint64,
	videoTrack format.Format,
	audioTrack format.Format,
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

	// avoid a crash by skipping invalid durations
	if du == 0 {
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

func (m *muxerVariantFMP4Segmenter) writeH26x(ntp time.Time, pts time.Duration, au [][]byte) error {
	randomAccessPresent := false

	switch m.videoTrack.(type) {
	case *format.H264:
		nonIDRPresent := false

		for _, nalu := range au {
			typ := h264.NALUType(nalu[0] & 0x1F)

			switch typ {
			case h264.NALUTypeIDR:
				randomAccessPresent = true

			case h264.NALUTypeNonIDR:
				nonIDRPresent = true
			}
		}

		if !randomAccessPresent && !nonIDRPresent {
			return nil
		}

	case *format.H265:
		for _, nalu := range au {
			typ := h265.NALUType((nalu[0] >> 1) & 0b111111)

			switch typ {
			case h265.NALUType_IDR_W_RADL, h265.NALUType_IDR_N_LP, h265.NALUType_CRA_NUT:
				randomAccessPresent = true
			}
		}
	}

	return m.writeH26xEntry(ntp, pts, au, randomAccessPresent)
}

func (m *muxerVariantFMP4Segmenter) writeH26xEntry(
	ntp time.Time,
	pts time.Duration,
	au [][]byte,
	randomAccessPresent bool,
) error {
	var dts time.Duration

	if !m.videoFirstRandomAccessReceived {
		// skip sample silently until we find one with an IDR
		if !randomAccessPresent {
			return nil
		}

		m.videoFirstRandomAccessReceived = true
		m.videoDTSExtractor = allocateDTSExtractor(m.videoTrack)
		m.lastVideoParams = extractVideoParams(m.videoTrack)

		var err error
		dts, err = m.videoDTSExtractor.Extract(au, pts)
		if err != nil {
			return fmt.Errorf("unable to extract DTS: %v", err)
		}

		m.startDTS = dts
		dts = 0
		pts -= m.startDTS
	} else {
		var err error
		dts, err = m.videoDTSExtractor.Extract(au, pts)
		if err != nil {
			return fmt.Errorf("unable to extract DTS: %v", err)
		}

		dts -= m.startDTS
		pts -= m.startDTS
	}

	avcc, err := h264.AVCCMarshal(au)
	if err != nil {
		return err
	}

	sample := &augmentedVideoSample{
		PartSample: fmp4.PartSample{
			PTSOffset:       int32(durationGoToMp4(pts-dts, 90000)),
			IsNonSyncSample: !randomAccessPresent,
			Payload:         avcc,
		},
		dts: dts,
		ntp: ntp,
	}

	// put samples into a queue in order to
	// - compute sample duration
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
			sample.ntp,
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
	if randomAccessPresent {
		videoParams := extractVideoParams(m.videoTrack)
		paramsChanged := !videoParamsEqual(m.lastVideoParams, videoParams)

		if (m.nextVideoSample.dts-m.currentSegment.startDTS) >= m.segmentDuration ||
			paramsChanged {
			err := m.currentSegment.finalize(m.nextVideoSample.dts)
			if err != nil {
				return err
			}
			m.onSegmentFinalized(m.currentSegment)

			m.firstSegmentFinalized = true

			m.currentSegment = newMuxerVariantFMP4Segment(
				m.lowLatency,
				m.genSegmentID(),
				m.nextVideoSample.ntp,
				m.nextVideoSample.dts,
				m.segmentMaxSize,
				m.videoTrack,
				m.audioTrack,
				m.genPartID,
				m.onPartFinalized,
			)

			if paramsChanged {
				m.lastVideoParams = videoParams
				m.firstSegmentFinalized = false

				// reset adjusted part duration
				m.sampleDurations = make(map[time.Duration]struct{})
			}
		}
	}

	return nil
}

func (m *muxerVariantFMP4Segmenter) writeAudio(ntp time.Time, dts time.Duration, au []byte) error {
	if m.videoTrack != nil {
		// wait for the video track
		if !m.videoFirstRandomAccessReceived {
			return nil
		}

		dts -= m.startDTS
		if dts < 0 {
			return nil
		}
	}

	sample := &augmentedAudioSample{
		PartSample: fmp4.PartSample{
			Payload: au,
		},
		dts: dts,
		ntp: ntp,
	}

	// put samples into a queue in order to compute the sample duration
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
				sample.ntp,
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

	err := m.currentSegment.writeAudio(sample, m.partDuration)
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
			m.nextAudioSample.ntp,
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
