package core

import (
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
)

type streamMedia struct {
	formats map[formats.Format]*streamFormat
}

func newStreamMedia(udpMaxPayloadSize int,
	medi *media.Media,
	generateRTPPackets bool,
) (*streamMedia, error) {
	sm := &streamMedia{
		formats: make(map[formats.Format]*streamFormat),
	}

	for _, forma := range medi.Formats {
		var err error
		sm.formats[forma], err = newStreamFormat(udpMaxPayloadSize, forma, generateRTPPackets)
		if err != nil {
			return nil, err
		}
	}

	return sm, nil
}
