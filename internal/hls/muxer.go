package hls

import (
	"bytes"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"
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

	h264SPS       []byte
	h264PPS       []byte
	aacConfig     rtpaac.MPEG4AudioConfig
	videoDTSEst   *h264.DTSEstimator
	audioAUCount  int
	tsCurrent     *tsFile
	tsQueue       []*tsFile
	tsByName      map[string]*tsFile
	tsDeleteCount int
	mutex         sync.RWMutex
	startPCR      time.Time
	startPTS      time.Duration
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
		tsCurrent:          newTSFile(videoTrack, audioTrack),
		tsByName:           make(map[string]*tsFile),
	}

	m.tsByName[m.tsCurrent.name] = m.tsCurrent
	m.tsQueue = append(m.tsQueue, m.tsCurrent)

	return m, nil
}

// Close closes a Muxer.
func (m *Muxer) Close() {
	m.tsCurrent.close()
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
	if !m.tsCurrent.firstPacketWritten && !idrPresent {
		return nil
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if idrPresent &&
		m.tsCurrent.firstPacketWritten &&
		m.tsCurrent.duration() >= m.hlsSegmentDuration {
		m.tsCurrent.close()

		m.tsCurrent = newTSFile(m.videoTrack, m.audioTrack)
		m.tsCurrent.setStartPCR(m.startPCR)

		m.tsByName[m.tsCurrent.name] = m.tsCurrent
		m.tsQueue = append(m.tsQueue, m.tsCurrent)
		if len(m.tsQueue) > m.hlsSegmentCount {
			delete(m.tsByName, m.tsQueue[0].name)
			m.tsQueue = m.tsQueue[1:]
			m.tsDeleteCount++
		}
	} else if !m.tsCurrent.firstPacketWritten {
		m.startPCR = time.Now()
		m.startPTS = pts
		m.tsCurrent.setStartPCR(m.startPCR)
	}

	pts = pts + ptsOffset - m.startPTS
	err := m.tsCurrent.writeH264(
		m.h264SPS,
		m.h264PPS,
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
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.videoTrack == nil {
		if m.audioAUCount >= segmentMinAUCount &&
			m.tsCurrent.firstPacketWritten &&
			m.tsCurrent.duration() >= m.hlsSegmentDuration {
			m.tsCurrent.close()

			m.audioAUCount = 0

			m.tsCurrent = newTSFile(m.videoTrack, m.audioTrack)
			m.tsCurrent.setStartPCR(m.startPCR)

			m.tsByName[m.tsCurrent.name] = m.tsCurrent
			m.tsQueue = append(m.tsQueue, m.tsCurrent)
			if len(m.tsQueue) > m.hlsSegmentCount {
				delete(m.tsByName, m.tsQueue[0].name)
				m.tsQueue = m.tsQueue[1:]
				m.tsDeleteCount++
			}
		} else if !m.tsCurrent.firstPacketWritten {
			m.startPCR = time.Now()
			m.startPTS = pts
			m.tsCurrent.setStartPCR(m.startPCR)
		}
	} else {
		if !m.tsCurrent.firstPacketWritten {
			return nil
		}
	}

	pts = pts + ptsOffset - m.startPTS

	for i, au := range aus {
		auPTS := pts + time.Duration(i)*1000*time.Second/time.Duration(m.aacConfig.SampleRate)

		err := m.tsCurrent.writeAAC(
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

// Playlist returns a reader to read the playlist.
func (m *Muxer) Playlist() io.Reader {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if len(m.tsQueue) == 0 {
		return nil
	}

	cnt := "#EXTM3U\n"
	cnt += "#EXT-X-VERSION:3\n"
	cnt += "#EXT-X-ALLOW-CACHE:NO\n"

	targetDuration := func() uint {
		ret := uint(math.Ceil(m.hlsSegmentDuration.Seconds()))

		// EXTINF, when rounded to the nearest integer, must be <= EXT-X-TARGETDURATION
		for _, f := range m.tsQueue {
			v2 := uint(math.Round(f.duration().Seconds()))
			if v2 > ret {
				ret = v2
			}
		}

		return ret
	}()
	cnt += "#EXT-X-TARGETDURATION:" + strconv.FormatUint(uint64(targetDuration), 10) + "\n"

	cnt += "#EXT-X-MEDIA-SEQUENCE:" + strconv.FormatInt(int64(m.tsDeleteCount), 10) + "\n"

	for _, f := range m.tsQueue {
		cnt += "#EXTINF:" + strconv.FormatFloat(f.duration().Seconds(), 'f', -1, 64) + ",\n"
		cnt += f.name + ".ts\n"
	}

	return bytes.NewReader([]byte(cnt))
}

// TSFile returns a reader to read a MPEG-TS file.
func (m *Muxer) TSFile(fname string) io.Reader {
	base := strings.TrimSuffix(fname, ".ts")

	m.mutex.RLock()
	f, ok := m.tsByName[base]
	m.mutex.RUnlock()

	if !ok {
		return nil
	}

	return f.newReader()
}
