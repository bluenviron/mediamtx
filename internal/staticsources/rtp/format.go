package rtp

import (
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/rtpreceiver"
	"github.com/pion/rtcp"
)

type rtpFormat struct {
	desc format.Format

	rtpReceiver *rtpreceiver.Receiver
}

func (f *rtpFormat) initialize() {
	f.rtpReceiver = &rtpreceiver.Receiver{
		ClockRate:            f.desc.ClockRate(),
		UnrealiableTransport: true,
		Period:               10 * time.Second,
		WritePacketRTCP: func(_ rtcp.Packet) {
		},
	}
}
