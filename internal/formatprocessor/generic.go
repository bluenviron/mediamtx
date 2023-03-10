package formatprocessor

import (
	"fmt"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/pion/rtp"
)

const (
	// 1500 (UDP MTU) - 20 (IP header) - 8 (UDP header)
	maxPacketSize = 1472
)

// UnitGeneric is a generic data unit.
type UnitGeneric struct {
	RTPPackets []*rtp.Packet
	NTP        time.Time
}

// GetRTPPackets implements Unit.
func (d *UnitGeneric) GetRTPPackets() []*rtp.Packet {
	return d.RTPPackets
}

// GetNTP implements Unit.
func (d *UnitGeneric) GetNTP() time.Time {
	return d.NTP
}

type formatProcessorGeneric struct{}

func newGeneric(forma format.Format, generateRTPPackets bool) (*formatProcessorGeneric, error) {
	if generateRTPPackets {
		return nil, fmt.Errorf("we don't know how to generate RTP packets of format %+v", forma)
	}

	return &formatProcessorGeneric{}, nil
}

func (t *formatProcessorGeneric) Process(unit Unit, hasNonRTSPReaders bool) error {
	tunit := unit.(*UnitGeneric)

	pkt := tunit.RTPPackets[0]

	// remove padding
	pkt.Header.Padding = false
	pkt.PaddingSize = 0

	if pkt.MarshalSize() > maxPacketSize {
		return fmt.Errorf("payload size (%d) is greater than maximum allowed (%d)",
			pkt.MarshalSize(), maxPacketSize)
	}

	return nil
}
