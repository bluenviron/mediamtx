package stream

import (
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"

	"github.com/bluenviron/mediamtx/internal/logger"
)

type streamMedia struct {
	UDPMaxPayloadSize  int
	Media              *description.Media
	GenerateRTPPackets bool
	DecodeErrLogger    logger.Writer

	formats  map[format.Format]*streamFormat
	gopCache bool
}

func (sm *streamMedia) initialize() error {
	sm.formats = make(map[format.Format]*streamFormat)

	for _, forma := range sm.Media.Formats {
		sf := &streamFormat{
			udpMaxPayloadSize:  sm.UDPMaxPayloadSize,
			format:             forma,
			generateRTPPackets: sm.GenerateRTPPackets,
			decodeErrLogger:    sm.DecodeErrLogger,
			gopCache:           sm.gopCache,
		}
		err := sf.initialize()
		if err != nil {
			return err
		}
		sm.formats[forma] = sf
	}

	return nil
}
