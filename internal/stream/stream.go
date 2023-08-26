// Package stream contains the Stream object.
package stream

import (
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// Stream is a media stream.
// It stores tracks, readers and allow to write data to readers.
type Stream struct {
	desc          *description.Session
	bytesReceived *uint64

	smedias     map[*description.Media]*streamMedia
	mutex       sync.RWMutex
	rtspStream  *gortsplib.ServerStream
	rtspsStream *gortsplib.ServerStream
}

// New allocates a Stream.
func New(
	udpMaxPayloadSize int,
	desc *description.Session,
	generateRTPPackets bool,
	bytesReceived *uint64,
	source logger.Writer,
) (*Stream, error) {
	s := &Stream{
		bytesReceived: bytesReceived,
		desc:          desc,
	}

	s.smedias = make(map[*description.Media]*streamMedia)

	for _, media := range desc.Medias {
		var err error
		s.smedias[media], err = newStreamMedia(udpMaxPayloadSize, media, generateRTPPackets, source)
		if err != nil {
			return nil, err
		}
	}

	return s, nil
}

// Close closes all resources of the stream.
func (s *Stream) Close() {
	if s.rtspStream != nil {
		s.rtspStream.Close()
	}
	if s.rtspsStream != nil {
		s.rtspsStream.Close()
	}
}

// Desc returns description of the stream.
func (s *Stream) Desc() *description.Session {
	return s.desc
}

// RTSPStream returns the RTSP stream.
func (s *Stream) RTSPStream(server *gortsplib.Server) *gortsplib.ServerStream {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.rtspStream == nil {
		s.rtspStream = gortsplib.NewServerStream(server, s.desc)
	}
	return s.rtspStream
}

// RTSPSStream returns the RTSPS stream.
func (s *Stream) RTSPSStream(server *gortsplib.Server) *gortsplib.ServerStream {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.rtspsStream == nil {
		s.rtspsStream = gortsplib.NewServerStream(server, s.desc)
	}
	return s.rtspsStream
}

// AddReader adds a reader.
func (s *Stream) AddReader(r interface{}, medi *description.Media, forma format.Format, cb func(unit.Unit)) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	sm := s.smedias[medi]
	sf := sm.formats[forma]
	sf.addReader(r, cb)
}

// RemoveReader removes a reader.
func (s *Stream) RemoveReader(r interface{}) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for _, sm := range s.smedias {
		for _, sf := range sm.formats {
			sf.removeReader(r)
		}
	}
}

// WriteUnit writes a Unit.
func (s *Stream) WriteUnit(medi *description.Media, forma format.Format, data unit.Unit) {
	sm := s.smedias[medi]
	sf := sm.formats[forma]

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	sf.writeUnit(s, medi, data)
}

// WriteRTPPacket writes a RTP packet.
func (s *Stream) WriteRTPPacket(
	medi *description.Media,
	forma format.Format,
	pkt *rtp.Packet,
	ntp time.Time,
	pts time.Duration,
) {
	sm := s.smedias[medi]
	sf := sm.formats[forma]

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	sf.writeRTPPacket(s, medi, pkt, ntp, pts)
}
