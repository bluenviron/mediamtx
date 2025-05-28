package formatprocessor

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type generic struct {
	UDPMaxPayloadSize  int
	Format             format.Format
	GenerateRTPPackets bool
	Parent             logger.Writer
}

func (t *generic) initialize() error {
	if t.GenerateRTPPackets {
		return fmt.Errorf("we don't know how to generate RTP packets of format %T", t.Format)
	}

	return nil
}

func (t *generic) ProcessUnit(_ unit.Unit) error {
	return fmt.Errorf("using a generic unit without RTP is not supported")
}

func (t *generic) ProcessRTPPacket(
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
	pkt.Padding = false
	pkt.PaddingSize = 0

	if pkt.MarshalSize() > t.UDPMaxPayloadSize {
		return nil, fmt.Errorf("payload size (%d) is greater than maximum allowed (%d)",
			pkt.MarshalSize(), t.UDPMaxPayloadSize)
	}

	return u, nil
}
