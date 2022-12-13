package core

import (
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/media"
)

type streamMedia struct {
	formats map[format.Format]*streamFormat
}

func newStreamMedia(medi *media.Media, generateRTPPackets bool) (*streamMedia, error) {
	sm := &streamMedia{
		formats: make(map[format.Format]*streamFormat),
	}

	for _, forma := range medi.Formats {
		var err error
		sm.formats[forma], err = newStreamFormat(forma, generateRTPPackets)
		if err != nil {
			return nil, err
		}
	}

	return sm, nil
}
