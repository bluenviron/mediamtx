package formatprocessor

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/unit"
)

type formatProcessorGeneric struct {
	udpMaxPayloadSize int
}

func newGeneric(
	udpMaxPayloadSize int,
	forma format.Format,
	generateRTPPackets bool,
) (*formatProcessorGeneric, error) {
	if generateRTPPackets {
		return nil, fmt.Errorf("we don't know how to generate RTP packets of format %T", forma)
	}

	return &formatProcessorGeneric{
		udpMaxPayloadSize: udpMaxPayloadSize,
	}, nil
}

func (t *formatProcessorGeneric) ProcessUnit(_ unit.Unit) error {
	return fmt.Errorf("using a generic unit without RTP is not supported")
}

func (t *formatProcessorGeneric) ProcessRTPPacket(
	pkt *rtp.Packet,
	ntp time.Time,
	pts int64,
	_ bool,
) (unit.Unit, error) {
	u := &unit.Generic{
		Base: unit.Base{
			RTPPackets: []*rtp.Packet{pkt},
			NTP:        ntp,
			PTS:        pts,
		},
	}

	// remove padding
	pkt.Header.Padding = false
	pkt.PaddingSize = 0

	if pkt.MarshalSize() > t.udpMaxPayloadSize {
		return nil, fmt.Errorf("payload size (%d) is greater than maximum allowed (%d)",
			pkt.MarshalSize(), t.udpMaxPayloadSize)
	}

	return u, nil
}
