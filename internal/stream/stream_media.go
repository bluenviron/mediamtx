package stream

import (
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"

	"github.com/bluenviron/mediamtx/internal/logger"
)

type streamMedia struct {
	formats map[format.Format]*streamFormat
}

func newStreamMedia(udpMaxPayloadSize int,
	medi *description.Media,
	generateRTPPackets bool,
	decodeErrLogger logger.Writer,
) (*streamMedia, error) {
	sm := &streamMedia{
		formats: make(map[format.Format]*streamFormat),
	}

	for _, forma := range medi.Formats {
		var err error
		sm.formats[forma], err = newStreamFormat(udpMaxPayloadSize, forma, generateRTPPackets, decodeErrLogger)
		if err != nil {
			return nil, err
		}
	}

	return sm, nil
}
