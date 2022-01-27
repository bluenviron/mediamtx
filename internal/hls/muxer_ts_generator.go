package hls

import (
	"log"
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

type muxerTSGenerator struct {
	hlsSegmentCount    int
	hlsSegmentDuration time.Duration
	videoTrack         *gortsplib.Track
	audioTrack         *gortsplib.Track
	h264Conf           *gortsplib.TrackConfigH264
	aacConf            *gortsplib.TrackConfigAAC
	streamPlaylist     *muxerStreamPlaylist

	writer         *muxerTSWriter
	currentSegment *muxerTSSegment
	videoDTSEst    *h264.DTSEstimator
	audioAUCount   int
	startPCR       time.Time
	startPTS       time.Duration
	segmentCounter int
}

func newMuxerTSGenerator(
	hlsSegmentCount int,
	hlsSegmentDuration time.Duration,
	videoTrack *gortsplib.Track,
	audioTrack *gortsplib.Track,
	h264Conf *gortsplib.TrackConfigH264,
	aacConf *gortsplib.TrackConfigAAC,
	streamPlaylist *muxerStreamPlaylist,
) *muxerTSGenerator {
	m := &muxerTSGenerator{
		hlsSegmentCount:    hlsSegmentCount,
		hlsSegmentDuration: hlsSegmentDuration,
		videoTrack:         videoTrack,
		audioTrack:         audioTrack,
		h264Conf:           h264Conf,
		aacConf:            aacConf,
		streamPlaylist:     streamPlaylist,
		writer:             newMuxerTSWriter(videoTrack, audioTrack),
	}

	m.currentSegment = newMuxerTSSegment(m.videoTrack, m.writer)

	return m
}

func (m *muxerTSGenerator) writeH264(pts time.Duration, nalus [][]byte) error {
	idrPresent := func() bool {
		for _, nalu := range nalus {
			typ := h264.NALUType(nalu[0] & 0x1F)
			if typ == h264.NALUTypeIDR {
				return true
			}
		}
		return false
	}()

	// skip group silently until we find one with a IDR
	if !m.currentSegment.firstPacketWritten && !idrPresent {
		return nil
	}

	if !m.currentSegment.firstPacketWritten {
		m.startPCR = time.Now()
		m.startPTS = pts
		m.videoDTSEst = h264.NewDTSEstimator()
	}

	dts := m.videoDTSEst.Feed(pts-m.startPTS) + pcrOffset
	pts = pts - m.startPTS + pcrOffset

	// switch segment or initialize the first segment
	if m.currentSegment.firstPacketWritten {
		if idrPresent &&
			(pts-m.currentSegment.startPTS) >= m.hlsSegmentDuration {
			m.currentSegment.endPTS = pts
			m.streamPlaylist.pushSegment(m.currentSegment)
			m.currentSegment = newMuxerTSSegment(m.videoTrack, m.writer)
			m.segmentCounter = 0
		} else {
			m.segmentCounter++
			if m.segmentCounter >= 500 {
				log.Println("SC", m.segmentCounter)
			}
		}
	}

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
			filteredNALUs = append(filteredNALUs, m.h264Conf.SPS, m.h264Conf.PPS)
		}

		filteredNALUs = append(filteredNALUs, nalu)
	}

	enc, err := h264.EncodeAnnexB(filteredNALUs)
	if err != nil {
		return err
	}

	return m.currentSegment.writeH264(m.startPCR, dts, pts, idrPresent, enc)
}

func (m *muxerTSGenerator) writeAAC(pts time.Duration, aus [][]byte) error {
	if m.videoTrack == nil && !m.currentSegment.firstPacketWritten {
		m.startPCR = time.Now()
		m.startPTS = pts
	}

	pts = pts - m.startPTS + pcrOffset

	// switch segment or initialize the first segment
	if m.videoTrack == nil {
		if m.currentSegment.firstPacketWritten {
			if m.audioAUCount >= segmentMinAUCount &&
				(pts-m.currentSegment.startPTS) >= m.hlsSegmentDuration {
				m.audioAUCount = 0
				m.currentSegment.endPTS = pts
				m.streamPlaylist.pushSegment(m.currentSegment)
				m.currentSegment = newMuxerTSSegment(m.videoTrack, m.writer)
			}
		}
	} else {
		if !m.currentSegment.firstPacketWritten {
			return nil
		}
	}

	pkts := make([]*aac.ADTSPacket, len(aus))

	for i, au := range aus {
		pkts[i] = &aac.ADTSPacket{
			Type:         m.aacConf.Type,
			SampleRate:   m.aacConf.SampleRate,
			ChannelCount: m.aacConf.ChannelCount,
			AU:           au,
		}
	}

	enc, err := aac.EncodeADTS(pkts)
	if err != nil {
		return err
	}

	err = m.currentSegment.writeAAC(m.startPCR, pts, enc)
	if err != nil {
		return err
	}

	if m.videoTrack == nil {
		m.audioAUCount += len(aus)
	}

	return nil
}
