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

	tm             *astits.Muxer
	currentSegment *muxerTSSegment
	videoDTSEst    *h264.DTSEstimator
	audioAUCount   int
	startPCR       time.Time
	startPTS       time.Duration
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
		streamPlaylist:     streamPlaylist,
		h264Conf:           h264Conf,
		aacConf:            aacConf,
	}

	m.tm = astits.NewMuxer(context.Background(), m)

	if videoTrack != nil {
		m.tm.AddElementaryStream(astits.PMTElementaryStream{
			ElementaryPID: 256,
			StreamType:    astits.StreamTypeH264Video,
		})
	}

	if audioTrack != nil {
		m.tm.AddElementaryStream(astits.PMTElementaryStream{
			ElementaryPID: 257,
			StreamType:    astits.StreamTypeAACAudio,
		})
	}

	if videoTrack != nil {
		m.tm.SetPCRPID(256)
	} else {
		m.tm.SetPCRPID(257)
	}

	m.currentSegment = newMuxerTSSegment(m.videoTrack, m)

	return m
}

func (m *muxerTSGenerator) Write(p []byte) (int, error) {
	return m.currentSegment.write(p)
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

	// switch segment or initialize the first segment
	if m.currentSegment.firstPacketWritten {
		if idrPresent &&
			m.currentSegment.duration() >= m.hlsSegmentDuration {
			m.streamPlaylist.pushSegment(m.currentSegment)
			m.currentSegment = newMuxerTSSegment(m.videoTrack, m)
		}
	} else {
		m.startPCR = time.Now()
		m.startPTS = pts
		m.videoDTSEst = h264.NewDTSEstimator()
	}

	dts := m.videoDTSEst.Feed(pts-m.startPTS) + pcrOffset
	pts = pts - m.startPTS + pcrOffset

	// prepend an AUD. This is required by video.js and iOS
	filteredNALUs := [][]byte{
		{byte(h264.NALUTypeAccessUnitDelimiter), 240},
	}

	for _, nalu := range nalus {
		// remove existing SPS, PPS, AUD
		typ := h264.NALUType(nalu[0] & 0x1F)
		switch typ {
		case h264.NALUTypeSPS, h264.NALUTypePPS, h264.NALUTypeAccessUnitDelimiter:
			continue
		}

		// add SPS and PPS before every IDR
		if typ == h264.NALUTypeIDR {
			filteredNALUs = append(filteredNALUs, m.h264Conf.SPS)
			filteredNALUs = append(filteredNALUs, m.h264Conf.PPS)
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
	// switch segment or initialize the first segment
	if m.videoTrack == nil {
		if m.currentSegment.firstPacketWritten {
			if m.audioAUCount >= segmentMinAUCount &&
				m.currentSegment.duration() >= m.hlsSegmentDuration {
				m.audioAUCount = 0
				m.streamPlaylist.pushSegment(m.currentSegment)
				m.currentSegment = newMuxerTSSegment(m.videoTrack, m)
			}
		} else {
			m.startPCR = time.Now()
			m.startPTS = pts
		}
	} else {
		if !m.currentSegment.firstPacketWritten {
			return nil
		}
	}

	pts = pts - m.startPTS + pcrOffset

	for _, au := range aus {
		enc, err := aac.EncodeADTS([]*aac.ADTSPacket{
			{
				SampleRate:   m.aacConf.SampleRate,
				ChannelCount: m.aacConf.ChannelCount,
				AU:           au,
			},
		})
		if err != nil {
			return err
		}

		err = m.currentSegment.writeAAC(m.startPCR, pts, enc)
		if err != nil {
			return err
		}

		if m.videoTrack == nil {
			m.audioAUCount++
		}

		pts += 1000 * time.Second / time.Duration(m.aacConf.SampleRate)
	}

	return nil
}
