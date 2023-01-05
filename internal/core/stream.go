package core

import (
	"github.com/aler9/gortsplib/v2"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/media"

	"github.com/aler9/rtsp-simple-server/internal/formatprocessor"
)

type stream struct {
	bytesReceived *uint64
	rtspStream    *gortsplib.ServerStream
	smedias       map[*media.Media]*streamMedia
}

func newStream(
	medias media.Medias,
	generateRTPPackets bool,
	bytesReceived *uint64,
) (*stream, error) {
	s := &stream{
		bytesReceived: bytesReceived,
		rtspStream:    gortsplib.NewServerStream(medias),
	}

	s.smedias = make(map[*media.Media]*streamMedia)

	for _, media := range s.rtspStream.Medias() {
		var err error
		s.smedias[media], err = newStreamMedia(media, generateRTPPackets)
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

func (s *stream) readerAdd(r reader, medi *media.Media, forma format.Format, cb func(formatprocessor.Data)) {
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

func (s *stream) writeData(medi *media.Media, forma format.Format, data formatprocessor.Data) error {
	sm := s.smedias[medi]
	sf := sm.formats[forma]
	return sf.writeData(s, medi, data)
}
