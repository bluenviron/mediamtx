package hls

import (
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/aac"
	"github.com/aler9/gortsplib/pkg/h264"
)

const (
	// an offset between PCR and PTS/DTS is needed to avoid PCR > PTS
	pcrOffset = 500 * time.Millisecond

	segmentMinAUCount = 100
)

func idrPresent(nalus [][]byte) bool {
	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)
		if typ == h264.NALUTypeIDR {
			return true
		}
	}
	return false
}

type muxerTSGenerator struct {
	hlsSegmentCount    int
	hlsSegmentDuration time.Duration
	hlsSegmentMaxSize  uint64
	videoTrack         *gortsplib.TrackH264
	audioTrack         *gortsplib.TrackAAC
	streamPlaylist     *muxerStreamPlaylist

	writer         *muxerTSWriter
	currentSegment *muxerTSSegment
	videoDTSEst    *h264.DTSEstimator
	startPCR       time.Time
	startPTS       time.Duration
}

func newMuxerTSGenerator(
	hlsSegmentCount int,
	hlsSegmentDuration time.Duration,
	hlsSegmentMaxSize uint64,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
	streamPlaylist *muxerStreamPlaylist,
) *muxerTSGenerator {
	m := &muxerTSGenerator{
		hlsSegmentCount:    hlsSegmentCount,
		hlsSegmentDuration: hlsSegmentDuration,
		hlsSegmentMaxSize:  hlsSegmentMaxSize,
		videoTrack:         videoTrack,
		audioTrack:         audioTrack,
		streamPlaylist:     streamPlaylist,
		writer:             newMuxerTSWriter(videoTrack, audioTrack),
	}

	return m
}

func (m *muxerTSGenerator) writeH264(pts time.Duration, nalus [][]byte) error {
	idrPresent := idrPresent(nalus)

	if m.currentSegment == nil {
		// skip groups silently until we find one with a IDR
		if !idrPresent {
			return nil
		}

		// create first segment
		m.currentSegment = newMuxerTSSegment(m.hlsSegmentMaxSize, m.videoTrack, m.writer)
		m.startPCR = time.Now()
		m.startPTS = pts
		m.videoDTSEst = h264.NewDTSEstimator()
		pts = pcrOffset
	} else {
		pts = pts - m.startPTS + pcrOffset

		// switch segment
		if idrPresent &&
			m.currentSegment.startPTS != nil &&
			(pts-*m.currentSegment.startPTS) >= m.hlsSegmentDuration {
			m.currentSegment.endPTS = pts
			m.streamPlaylist.pushSegment(m.currentSegment)
			m.currentSegment = newMuxerTSSegment(m.hlsSegmentMaxSize, m.videoTrack, m.writer)
		}
	}

	dts := m.videoDTSEst.Feed(pts-m.startPTS) + pcrOffset

	// prepend an AUD. This is required by video.js and iOS
	filteredNALUs := [][]byte{
		{byte(h264.NALUTypeAccessUnitDelimiter), 240},
	}

	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)
		switch typ {
		case h264.NALUTypeSPS, h264.NALUTypePPS, h264.NALUTypeAccessUnitDelimiter:
			// remove existing SPS, PPS, AUD
			continue

		case h264.NALUTypeIDR:
			// add SPS and PPS before every IDR
			filteredNALUs = append(filteredNALUs, m.videoTrack.SPS(), m.videoTrack.PPS())
		}

		filteredNALUs = append(filteredNALUs, nalu)
	}

	enc, err := h264.EncodeAnnexB(filteredNALUs)
	if err != nil {
		if m.currentSegment.buf.Len() > 0 {
			m.streamPlaylist.pushSegment(m.currentSegment)
		}
		m.currentSegment = nil
		return err
	}

	err = m.currentSegment.writeH264(m.startPCR, dts, pts, idrPresent, enc)
	if err != nil {
		if m.currentSegment.buf.Len() > 0 {
			m.streamPlaylist.pushSegment(m.currentSegment)
		}
		m.currentSegment = nil
		return err
	}

	return nil
}

func (m *muxerTSGenerator) writeAAC(pts time.Duration, aus [][]byte) error {
	if m.videoTrack == nil {
		if m.currentSegment == nil {
			// create first segment
			m.currentSegment = newMuxerTSSegment(m.hlsSegmentMaxSize, m.videoTrack, m.writer)
			m.startPCR = time.Now()
			m.startPTS = pts
			pts = pcrOffset
		} else {
			pts = pts - m.startPTS + pcrOffset

			// switch segment
			if m.currentSegment.audioAUCount >= segmentMinAUCount &&
				m.currentSegment.startPTS != nil &&
				(pts-*m.currentSegment.startPTS) >= m.hlsSegmentDuration {
				m.currentSegment.endPTS = pts
				m.streamPlaylist.pushSegment(m.currentSegment)
				m.currentSegment = newMuxerTSSegment(m.hlsSegmentMaxSize, m.videoTrack, m.writer)
			}
		}
	} else {
		// wait for the video track
		if m.currentSegment == nil {
			return nil
		}

		pts = pts - m.startPTS + pcrOffset
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

	err = m.currentSegment.writeAAC(m.startPCR, pts, enc, len(aus))
	if err != nil {
		if m.currentSegment.buf.Len() > 0 {
			m.streamPlaylist.pushSegment(m.currentSegment)
		}
		m.currentSegment = nil
		return err
	}

	return nil
}
