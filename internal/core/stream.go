package core

import (
	"time"

	"github.com/bluenviron/gortsplib/v3"
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/formatprocessor"
)

type stream struct {
	bytesReceived *uint64

	rtspStream *gortsplib.ServerStream
	smedias    map[*media.Media]*streamMedia
}

func newStream(
	udpMaxPayloadSize int,
	medias media.Medias,
	generateRTPPackets bool,
	bytesReceived *uint64,
	source source,
) (*stream, error) {
	s := &stream{
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

func (s *stream) close() {
	s.rtspStream.Close()
}

func (s *stream) medias() media.Medias {
	return s.rtspStream.Medias()
}

func (s *stream) readerAdd(r reader, medi *media.Media, forma formats.Format, cb func(formatprocessor.Unit)) {
	sm := s.smedias[medi]
	sf := sm.formats[forma]
	sf.readerAdd(r, cb)
}

func (s *stream) readerRemove(r reader) {
	for _, sm := range s.smedias {
		for _, sf := range sm.formats {
			sf.readerRemove(r)
		}
	}
}

func (s *stream) writeUnit(medi *media.Media, forma formats.Format, data formatprocessor.Unit) {
	sm := s.smedias[medi]
	sf := sm.formats[forma]
	sf.writeUnit(s, medi, data)
}

func (s *stream) writeRTPPacket(medi *media.Media, forma formats.Format, pkt *rtp.Packet, ntp time.Time) {
	sm := s.smedias[medi]
	sf := sm.formats[forma]
	sf.writeRTPPacket(s, medi, pkt, ntp)
}
