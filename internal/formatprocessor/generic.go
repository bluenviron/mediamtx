package formatprocessor

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
)

// UnitGeneric is a generic data unit.
type UnitGeneric struct {
	BaseUnit
}

type formatProcessorGeneric struct {
	udpMaxPayloadSize int
}

func newGeneric(
	udpMaxPayloadSize int,
	forma formats.Format,
	generateRTPPackets bool,
	_ logger.Writer,
) (*formatProcessorGeneric, error) {
	if generateRTPPackets {
		return nil, fmt.Errorf("we don't know how to generate RTP packets of format %+v", forma)
	}

	return &formatProcessorGeneric{
		udpMaxPayloadSize: udpMaxPayloadSize,
	}, nil
}

func (t *formatProcessorGeneric) Process(unit Unit, _ bool) error {
	tunit := unit.(*UnitGeneric)

	pkt := tunit.RTPPackets[0]

	// remove padding
	pkt.Header.Padding = false
	pkt.PaddingSize = 0

	if pkt.MarshalSize() > t.udpMaxPayloadSize {
		return fmt.Errorf("payload size (%d) is greater than maximum allowed (%d)",
			pkt.MarshalSize(), t.udpMaxPayloadSize)
	}

	return nil
}

func (t *formatProcessorGeneric) UnitForRTPPacket(pkt *rtp.Packet, ntp time.Time) Unit {
	return &UnitGeneric{
		BaseUnit: BaseUnit{
			RTPPackets: []*rtp.Packet{pkt},
			NTP:        ntp,
		},
	}
}
