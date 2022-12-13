package core

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

type dataGeneric struct {
	rtpPackets []*rtp.Packet
	ntp        time.Time
}

func (d *dataGeneric) getRTPPackets() []*rtp.Packet {
	return d.rtpPackets
}

func (d *dataGeneric) getNTP() time.Time {
	return d.ntp
}

type formatProcessorGeneric struct{}

func newFormatProcessorGeneric(forma format.Format, generateRTPPackets bool) (*formatProcessorGeneric, error) {
	if generateRTPPackets {
		return nil, fmt.Errorf("we don't know how to generate RTP packets of format %+v", forma)
	}

	return &formatProcessorGeneric{}, nil
}

func (t *formatProcessorGeneric) process(dat data, hasNonRTSPReaders bool) error {
	tdata := dat.(*dataGeneric)

	pkt := tdata.rtpPackets[0]

	// remove padding
	pkt.Header.Padding = false
	pkt.PaddingSize = 0

	if pkt.MarshalSize() > maxPacketSize {
		return fmt.Errorf("payload size (%d) is greater than maximum allowed (%d)",
			pkt.MarshalSize(), maxPacketSize)
	}

	return nil
}
