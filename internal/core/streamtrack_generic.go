package core

import (
	"fmt"
)

const (
	// 1500 (UDP MTU) - 20 (IP header) - 8 (UDP header)
	maxPacketSize = 1472
)

type streamTrackGeneric struct{}

func newStreamTrackGeneric() *streamTrackGeneric {
	return &streamTrackGeneric{}
}

func (t *streamTrackGeneric) onData(dat data, hasNonRTSPReaders bool) error {
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
