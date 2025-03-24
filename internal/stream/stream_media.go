package stream

import (
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediamtx/internal/counterdumper"
)

type streamMedia struct {
	UDPMaxPayloadSize  int
	Media              *description.Media
	GenerateRTPPackets bool
	DecodeErrors       *counterdumper.CounterDumper

	formats map[format.Format]*streamFormat
}

func (sm *streamMedia) initialize() error {
	sm.formats = make(map[format.Format]*streamFormat)

	for _, forma := range sm.Media.Formats {
		sf := &streamFormat{
			UDPMaxPayloadSize:  sm.UDPMaxPayloadSize,
			Format:             forma,
			GenerateRTPPackets: sm.GenerateRTPPackets,
			DecodeErrors:       sm.DecodeErrors,
		}
		err := sf.initialize()
		if err != nil {
			return err
		}
		sm.formats[forma] = sf
	}

	return nil
}
