package core

import (
	"bytes"
	"sync"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
)

type streamNonRTSPReadersMap struct {
	mutex sync.RWMutex
	ma    map[reader]struct{}
}

func newStreamNonRTSPReadersMap() *streamNonRTSPReadersMap {
	return &streamNonRTSPReadersMap{
		ma: make(map[reader]struct{}),
	}
}

func (m *streamNonRTSPReadersMap) close() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.ma = nil
}

func (m *streamNonRTSPReadersMap) add(r reader) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.ma[r] = struct{}{}
}

func (m *streamNonRTSPReadersMap) remove(r reader) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.ma, r)
}

func (m *streamNonRTSPReadersMap) forwardPacketRTP(data *data) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for c := range m.ma {
		c.onReaderData(data)
	}
}

type stream struct {
	nonRTSPReaders *streamNonRTSPReadersMap
	rtspStream     *gortsplib.ServerStream
}

func newStream(tracks gortsplib.Tracks) *stream {
	s := &stream{
		nonRTSPReaders: newStreamNonRTSPReadersMap(),
		rtspStream:     gortsplib.NewServerStream(tracks),
	}
	return s
}

func (s *stream) close() {
	s.nonRTSPReaders.close()
	s.rtspStream.Close()
}

func (s *stream) tracks() gortsplib.Tracks {
	return s.rtspStream.Tracks()
}

func (s *stream) readerAdd(r reader) {
	if _, ok := r.(pathRTSPSession); !ok {
		s.nonRTSPReaders.add(r)
	}
}

func (s *stream) readerRemove(r reader) {
	if _, ok := r.(pathRTSPSession); !ok {
		s.nonRTSPReaders.remove(r)
	}
}

func (s *stream) updateH264TrackParameters(h264track *gortsplib.TrackH264, nalus [][]byte) {
	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case h264.NALUTypeSPS:
			if !bytes.Equal(nalu, h264track.SafeSPS()) {
				h264track.SafeSetSPS(append([]byte(nil), nalu...))
			}

		case h264.NALUTypePPS:
			if !bytes.Equal(nalu, h264track.SafePPS()) {
				h264track.SafeSetPPS(append([]byte(nil), nalu...))
			}
		}
	}
}

// remux is needed to
// - fix corrupted streams
// - make streams compatible with all protocols
func (s *stream) remuxH264NALUs(h264track *gortsplib.TrackH264, data *data) {
	var filteredNALUs [][]byte //nolint:prealloc

	for _, nalu := range data.h264NALUs {
		typ := h264.NALUType(nalu[0] & 0x1F)
		switch typ {
		case h264.NALUTypeSPS, h264.NALUTypePPS:
			// remove since they're automatically added before every IDR
			continue

		case h264.NALUTypeAccessUnitDelimiter:
			// remove since it is not needed
			continue

		case h264.NALUTypeIDR:
			// add SPS and PPS before every IDR
			filteredNALUs = append(filteredNALUs, h264track.SafeSPS(), h264track.SafePPS())
		}

		filteredNALUs = append(filteredNALUs, nalu)
	}

	data.h264NALUs = filteredNALUs
}

func (s *stream) writeData(data *data) {
	track := s.rtspStream.Tracks()[data.trackID]
	if h264track, ok := track.(*gortsplib.TrackH264); ok {
		s.updateH264TrackParameters(h264track, data.h264NALUs)
		s.remuxH264NALUs(h264track, data)
	}

	// forward to RTSP readers
	s.rtspStream.WritePacketRTP(data.trackID, data.rtp, data.ptsEqualsDTS)

	// forward to non-RTSP readers
	s.nonRTSPReaders.forwardPacketRTP(data)
}
