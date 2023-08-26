package formatprocessor

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type formatProcessorGeneric struct {
	udpMaxPayloadSize int
}

func newGeneric(
	udpMaxPayloadSize int,
	forma format.Format,
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

func (t *formatProcessorGeneric) Process(u unit.Unit, _ bool) error {
	tunit := u.(*unit.Generic)

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

func (t *formatProcessorGeneric) UnitForRTPPacket(pkt *rtp.Packet, ntp time.Time, pts time.Duration) Unit {
	return &unit.Generic{
		Base: unit.Base{
			RTPPackets: []*rtp.Packet{pkt},
			NTP:        ntp,
			PTS:        pts,
		},
	}
}
