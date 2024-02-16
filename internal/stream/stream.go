// Package stream contains the Stream object.
package stream

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/asyncwriter"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// ReadFunc is the callback passed to AddReader().
type ReadFunc func(unit.Unit) error

// Stream is a media stream.
// It stores tracks, readers and allows to write data to readers.
type Stream struct {
	desc *description.Session

	bytesReceived *uint64
	bytesSent     *uint64
	smedias       map[*description.Media]*streamMedia
	mutex         sync.RWMutex
	rtspStream    *gortsplib.ServerStream
	rtspsStream   *gortsplib.ServerStream
}

// New allocates a Stream.
func New(
	udpMaxPayloadSize int,
	desc *description.Session,
	generateRTPPackets bool,
	decodeErrLogger logger.Writer,
) (*Stream, error) {
	s := &Stream{
		desc:          desc,
		bytesReceived: new(uint64),
		bytesSent:     new(uint64),
	}

	s.smedias = make(map[*description.Media]*streamMedia)

	for _, media := range desc.Medias {
		var err error
		s.smedias[media], err = newStreamMedia(udpMaxPayloadSize, media, generateRTPPackets, decodeErrLogger)
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

// Desc returns the description of the stream.
func (s *Stream) Desc() *description.Session {
	return s.desc
}

// BytesReceived returns received bytes.
func (s *Stream) BytesReceived() uint64 {
	return atomic.LoadUint64(s.bytesReceived)
}

// BytesSent returns sent bytes.
func (s *Stream) BytesSent() uint64 {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	bytesSent := atomic.LoadUint64(s.bytesSent)
	if s.rtspStream != nil {
		bytesSent += s.rtspStream.BytesSent()
	}
	if s.rtspsStream != nil {
		bytesSent += s.rtspsStream.BytesSent()
	}
	return bytesSent
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
func (s *Stream) AddReader(r *asyncwriter.Writer, medi *description.Media, forma format.Format, cb ReadFunc) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	sm := s.smedias[medi]
	sf := sm.formats[forma]
	sf.addReader(r, cb)
}

// RemoveReader removes a reader.
func (s *Stream) RemoveReader(r *asyncwriter.Writer) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for _, sm := range s.smedias {
		for _, sf := range sm.formats {
			sf.removeReader(r)
		}
	}
}

// FormatsForReader returns all formats that a reader is reading.
func (s *Stream) FormatsForReader(r *asyncwriter.Writer) []format.Format {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	var formats []format.Format

	for _, sm := range s.smedias {
		for forma, sf := range sm.formats {
			if _, ok := sf.readers[r]; ok {
				formats = append(formats, forma)
			}
		}
	}

	return formats
}

// WriteUnit writes a Unit.
func (s *Stream) WriteUnit(medi *description.Media, forma format.Format, u unit.Unit) {
	sm := s.smedias[medi]
	sf := sm.formats[forma]

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	sf.writeUnit(s, medi, u)
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
