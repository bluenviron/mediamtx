package stream

import (
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
)

type subStreamMedia struct {
	curMedia      *description.Media
	streamMedia   *streamMedia
	useRTPPackets bool

	formats map[format.Format]*subStreamFormat
}

func (ssm *subStreamMedia) initialize() error {
	ssm.formats = make(map[format.Format]*subStreamFormat)

	for i, curFormat := range ssm.curMedia.Formats {
		forma := ssm.streamMedia.media.Formats[i]

		ssf := &subStreamFormat{
			curFormat:     curFormat,
			streamFormat:  ssm.streamMedia.formats[forma],
			useRTPPackets: ssm.useRTPPackets,
		}
		err := ssf.initialize()
		if err != nil {
			return err
		}
		ssm.formats[curFormat] = ssf
	}

	return nil
}
