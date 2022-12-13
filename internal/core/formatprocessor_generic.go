package core

import (
	"fmt"

	"github.com/aler9/gortsplib/v2/pkg/format"
)

const (
	// 1500 (UDP MTU) - 20 (IP header) - 8 (UDP header)
	maxPacketSize = 1472
)

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
