package hls

import (
	"context"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/aac"
	"github.com/aler9/gortsplib/pkg/h264"
	"github.com/asticode/go-astits"
)

const (
	segmentMinAUCount = 100
)

type writerFunc func(p []byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) {
	return f(p)
}

type muxerVariantMPEGTSSegmenter struct {
	segmentCount    int
	segmentDuration time.Duration
	segmentMaxSize  uint64
	videoTrack      *gortsplib.TrackH264
	audioTrack      *gortsplib.TrackAAC
	onSegmentReady  func(*muxerVariantMPEGTSSegment)

	writer         *astits.Muxer
	currentSegment *muxerVariantMPEGTSSegment
	videoDTSEst    *h264.DTSEstimator
	startPCR       time.Time
	startPTS       time.Duration
}

func newMuxerVariantMPEGTSSegmenter(
	segmentCount int,
	segmentDuration time.Duration,
	segmentMaxSize uint64,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
	onSegmentReady func(*muxerVariantMPEGTSSegment),
) *muxerVariantMPEGTSSegmenter {
	m := &muxerVariantMPEGTSSegmenter{
		segmentCount:    segmentCount,
		segmentDuration: segmentDuration,
		segmentMaxSize:  segmentMaxSize,
		videoTrack:      videoTrack,
		audioTrack:      audioTrack,
		onSegmentReady:  onSegmentReady,
	}

	m.writer = astits.NewMuxer(
		context.Background(),
		writerFunc(func(p []byte) (int, error) {
			return m.currentSegment.write(p)
		}))

	if videoTrack != nil {
		m.writer.AddElementaryStream(astits.PMTElementaryStream{
			ElementaryPID: 256,
			StreamType:    astits.StreamTypeH264Video,
		})
	}

	if audioTrack != nil {
		m.writer.AddElementaryStream(astits.PMTElementaryStream{
			ElementaryPID: 257,
			StreamType:    astits.StreamTypeAACAudio,
		})
	}

	if videoTrack != nil {
		m.writer.SetPCRPID(256)
	} else {
		m.writer.SetPCRPID(257)
	}

	return m
}

func (m *muxerVariantMPEGTSSegmenter) writeH264(pts time.Duration, nalus [][]byte) error {
	now := time.Now()
	idrPresent := h264.IDRPresent(nalus)

	if m.currentSegment == nil {
		// skip groups silently until we find one with a IDR
		if !idrPresent {
			return nil
		}

		// create first segment
		m.currentSegment = newMuxerVariantMPEGTSSegment(now, m.segmentMaxSize,
			m.videoTrack, m.writer.WriteData)
		m.startPCR = now
		m.videoDTSEst = h264.NewDTSEstimator()
		m.startPTS = pts
		pts = 0
	} else {
		pts -= m.startPTS

		// switch segment
		if idrPresent &&
			m.currentSegment.startPTS != nil &&
			(pts-*m.currentSegment.startPTS) >= m.segmentDuration {
			m.currentSegment.endPTS = pts
			m.onSegmentReady(m.currentSegment)
			m.currentSegment = newMuxerVariantMPEGTSSegment(now, m.segmentMaxSize,
				m.videoTrack, m.writer.WriteData)
		}
	}

	dts := m.videoDTSEst.Feed(pts)

	// prepend an AUD. This is required by video.js and iOS
	nalus = append([][]byte{{byte(h264.NALUTypeAccessUnitDelimiter), 240}}, nalus...)

	enc, err := h264.AnnexBEncode(nalus)
	if err != nil {
		if m.currentSegment.buf.Len() > 0 {
			m.onSegmentReady(m.currentSegment)
		}
		m.currentSegment = nil
		return err
	}

	err = m.currentSegment.writeH264(now.Sub(m.startPCR), dts,
		pts, idrPresent, enc)
	if err != nil {
		if m.currentSegment.buf.Len() > 0 {
			m.onSegmentReady(m.currentSegment)
		}
		m.currentSegment = nil
		return err
	}

	return nil
}

func (m *muxerVariantMPEGTSSegmenter) writeAAC(pts time.Duration, aus [][]byte) error {
	now := time.Now()

	if m.videoTrack == nil {
		if m.currentSegment == nil {
			// create first segment
			m.currentSegment = newMuxerVariantMPEGTSSegment(now, m.segmentMaxSize,
				m.videoTrack, m.writer.WriteData)
			m.startPCR = now
			m.startPTS = pts
			pts = 0
		} else {
			pts -= m.startPTS

			// switch segment
			if m.currentSegment.audioAUCount >= segmentMinAUCount &&
				m.currentSegment.startPTS != nil &&
				(pts-*m.currentSegment.startPTS) >= m.segmentDuration {
				m.currentSegment.endPTS = pts
				m.onSegmentReady(m.currentSegment)
				m.currentSegment = newMuxerVariantMPEGTSSegment(now, m.segmentMaxSize,
					m.videoTrack, m.writer.WriteData)
			}
		}
	} else {
		// wait for the video track
		if m.currentSegment == nil {
			return nil
		}

		pts -= m.startPTS
	}

	pkts := make([]*aac.ADTSPacket, len(aus))

	for i, au := range aus {
		pkts[i] = &aac.ADTSPacket{
			Type:         m.audioTrack.Type(),
			SampleRate:   m.audioTrack.ClockRate(),
			ChannelCount: m.audioTrack.ChannelCount(),
			AU:           au,
		}
	}

	enc, err := aac.EncodeADTS(pkts)
	if err != nil {
		return err
	}

	err = m.currentSegment.writeAAC(now.Sub(m.startPCR), pts, enc, len(aus))
	if err != nil {
		if m.currentSegment.buf.Len() > 0 {
			m.onSegmentReady(m.currentSegment)
		}
		m.currentSegment = nil
		return err
	}

	return nil
}
