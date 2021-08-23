package hls

import (
	"io"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/rtpaac"

	"github.com/aler9/rtsp-simple-server/internal/h264"
)

const (
	// an offset is needed to
	// - avoid negative PTS values
	// - avoid PTS < DTS during startup
	ptsOffset = 2 * time.Second

	segmentMinAUCount = 100
)

// Muxer is a HLS muxer.
type Muxer struct {
	hlsSegmentCount    int
	hlsSegmentDuration time.Duration
	videoTrack         *gortsplib.Track
	audioTrack         *gortsplib.Track

	h264SPS         []byte
	h264PPS         []byte
	aacConfig       rtpaac.MPEG4AudioConfig
	videoDTSEst     *h264.DTSEstimator
	audioAUCount    int
	currentSegment  *segment
	startPCR        time.Time
	startPTS        time.Duration
	primaryPlaylist *primaryPlaylist
	streamPlaylist  *streamPlaylist
}

// NewMuxer allocates a Muxer.
func NewMuxer(
	hlsSegmentCount int,
	hlsSegmentDuration time.Duration,
	videoTrack *gortsplib.Track,
	audioTrack *gortsplib.Track) (*Muxer, error) {
	var h264SPS []byte
	var h264PPS []byte
	if videoTrack != nil {
		var err error
		h264SPS, h264PPS, err = videoTrack.ExtractDataH264()
		if err != nil {
			return nil, err
		}
	}

	var aacConfig rtpaac.MPEG4AudioConfig
	if audioTrack != nil {
		byts, err := audioTrack.ExtractDataAAC()
		if err != nil {
			return nil, err
		}

		err = aacConfig.Decode(byts)
		if err != nil {
			return nil, err
		}
	}

	m := &Muxer{
		hlsSegmentCount:    hlsSegmentCount,
		hlsSegmentDuration: hlsSegmentDuration,
		videoTrack:         videoTrack,
		audioTrack:         audioTrack,
		h264SPS:            h264SPS,
		h264PPS:            h264PPS,
		aacConfig:          aacConfig,
		videoDTSEst:        h264.NewDTSEstimator(),
		currentSegment:     newSegment(videoTrack, audioTrack, h264SPS, h264PPS),
		primaryPlaylist:    newPrimaryPlaylist(videoTrack, audioTrack, h264SPS, h264PPS),
		streamPlaylist:     newStreamPlaylist(hlsSegmentCount),
	}

	return m, nil
}

// Close closes a Muxer.
func (m *Muxer) Close() {
	m.streamPlaylist.close()
}

// WriteH264 writes H264 NALUs, grouped by PTS, into the muxer.
func (m *Muxer) WriteH264(pts time.Duration, nalus [][]byte) error {
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

	if m.currentSegment.firstPacketWritten {
		if idrPresent &&
			m.currentSegment.duration() >= m.hlsSegmentDuration {
			m.streamPlaylist.pushSegment(m.currentSegment)

			m.currentSegment = newSegment(m.videoTrack, m.audioTrack, m.h264SPS, m.h264PPS)
			m.currentSegment.setStartPCR(m.startPCR)
		}
	} else {
		m.startPCR = time.Now()
		m.startPTS = pts
		m.currentSegment.setStartPCR(m.startPCR)
	}

	pts = pts + ptsOffset - m.startPTS

	err := m.currentSegment.writeH264(
		m.videoDTSEst.Feed(pts),
		pts,
		idrPresent,
		nalus)
	if err != nil {
		return err
	}

	return nil
}

// WriteAAC writes AAC AUs, grouped by PTS, into the muxer.
func (m *Muxer) WriteAAC(pts time.Duration, aus [][]byte) error {
	if m.videoTrack == nil {
		if m.currentSegment.firstPacketWritten {
			if m.audioAUCount >= segmentMinAUCount &&
				m.currentSegment.duration() >= m.hlsSegmentDuration {
				m.audioAUCount = 0

				m.streamPlaylist.pushSegment(m.currentSegment)

				m.currentSegment = newSegment(m.videoTrack, m.audioTrack, m.h264SPS, m.h264PPS)
				m.currentSegment.setStartPCR(m.startPCR)
			}
		} else {
			m.startPCR = time.Now()
			m.startPTS = pts
			m.currentSegment.setStartPCR(m.startPCR)
		}
	} else {
		if !m.currentSegment.firstPacketWritten {
			return nil
		}
	}

	pts = pts + ptsOffset - m.startPTS

	for i, au := range aus {
		auPTS := pts + time.Duration(i)*1000*time.Second/time.Duration(m.aacConfig.SampleRate)

		err := m.currentSegment.writeAAC(
			m.aacConfig.SampleRate,
			m.aacConfig.ChannelCount,
			auPTS,
			au)
		if err != nil {
			return err
		}

		m.audioAUCount++
	}

	return nil
}

// PrimaryPlaylist returns a reader to read the primary playlist
func (m *Muxer) PrimaryPlaylist() io.Reader {
	return m.primaryPlaylist.reader()
}

// StreamPlaylist returns a reader to read the stream playlist.
func (m *Muxer) StreamPlaylist() io.Reader {
	return m.streamPlaylist.reader()
}

// Segment returns a reader to read a segment.
func (m *Muxer) Segment(fname string) io.Reader {
	return m.streamPlaylist.segment(fname)
}
