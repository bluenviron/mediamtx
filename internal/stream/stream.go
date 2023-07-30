// Package stream contains the Stream object.
package stream

import (
	"time"

	"github.com/bluenviron/gortsplib/v3"
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/formatprocessor"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// Stream is a media stream.
// It stores tracks, readers and allow to write data to readers.
type Stream struct {
	bytesReceived *uint64

	rtspStream *gortsplib.ServerStream
	smedias    map[*media.Media]*streamMedia
}

// New allocates a Stream.
func New(
	udpMaxPayloadSize int,
	medias media.Medias,
	generateRTPPackets bool,
	bytesReceived *uint64,
	source logger.Writer,
) (*Stream, error) {
	s := &Stream{
		bytesReceived: bytesReceived,
		rtspStream:    gortsplib.NewServerStream(medias),
	}

	s.smedias = make(map[*media.Media]*streamMedia)

	for _, media := range s.rtspStream.Medias() {
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
	s.rtspStream.Close()
}

// Medias returns medias of the stream.
func (s *Stream) Medias() media.Medias {
	return s.rtspStream.Medias()
}

// RTSPStream returns the RTSP stream.
func (s *Stream) RTSPStream() *gortsplib.ServerStream {
	return s.rtspStream
}

// AddReader adds a reader.
func (s *Stream) AddReader(r interface{}, medi *media.Media, forma formats.Format, cb func(formatprocessor.Unit)) {
	sm := s.smedias[medi]
	sf := sm.formats[forma]
	sf.addReader(r, cb)
}

// RemoveReader removes a reader.
func (s *Stream) RemoveReader(r interface{}) {
	for _, sm := range s.smedias {
		for _, sf := range sm.formats {
			sf.removeReader(r)
		}
	}
}

// WriteUnit writes a Unit.
func (s *Stream) WriteUnit(medi *media.Media, forma formats.Format, data formatprocessor.Unit) {
	sm := s.smedias[medi]
	sf := sm.formats[forma]
	sf.writeUnit(s, medi, data)
}

// WriteRTPPacket writes a RTP packet.
func (s *Stream) WriteRTPPacket(medi *media.Media, forma formats.Format, pkt *rtp.Packet, ntp time.Time) {
	sm := s.smedias[medi]
	sf := sm.formats[forma]
	sf.writeRTPPacket(s, medi, pkt, ntp)
}
