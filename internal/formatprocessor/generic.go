package formatprocessor

import (
	"fmt"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/pion/rtp"
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

type formatProcessorGeneric struct {
	udpMaxPayloadSize int
}

func newGeneric(
	udpMaxPayloadSize int,
	forma format.Format,
	generateRTPPackets bool,
) (*formatProcessorGeneric, error) {
	if generateRTPPackets {
		return nil, fmt.Errorf("we don't know how to generate RTP packets of format %+v", forma)
	}

	return &formatProcessorGeneric{
		udpMaxPayloadSize: udpMaxPayloadSize,
	}, nil
}

func (t *formatProcessorGeneric) Process(unit Unit, hasNonRTSPReaders bool) error {
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
