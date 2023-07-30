package stream

import (
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"

	"github.com/bluenviron/mediamtx/internal/logger"
)

type streamMedia struct {
	formats map[formats.Format]*streamFormat
}

func newStreamMedia(udpMaxPayloadSize int,
	medi *media.Media,
	generateRTPPackets bool,
	source logger.Writer,
) (*streamMedia, error) {
	sm := &streamMedia{
		formats: make(map[formats.Format]*streamFormat),
	}

	for _, forma := range medi.Formats {
		var err error
		sm.formats[forma], err = newStreamFormat(udpMaxPayloadSize, forma, generateRTPPackets, source)
		if err != nil {
			return nil, err
		}
	}

	return sm, nil
}
