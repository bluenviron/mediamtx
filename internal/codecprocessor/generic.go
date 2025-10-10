package codecprocessor

import (
	"fmt"

	"github.com/bluenviron/gortsplib/v5/pkg/format"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type generic struct {
	RTPMaxPayloadSize  int
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

func (t *generic) ProcessUnit(_ *unit.Unit) error {
	return fmt.Errorf("using a generic unit without RTP is not supported")
}

func (t *generic) ProcessRTPPacket(
	u *unit.Unit,
	_ bool,
) error {
	pkt := u.RTPPackets[0]

	// remove padding
	pkt.Padding = false
	pkt.PaddingSize = 0

	if len(pkt.Payload) > t.RTPMaxPayloadSize {
		return fmt.Errorf("RTP payload size (%d) is greater than maximum allowed (%d)",
			len(pkt.Payload), t.RTPMaxPayloadSize)
	}

	return nil
}
