package webrtc

import (
	"github.com/pion/webrtc/v4"
)

// OutgoingDataChannel is an outgoing data channel.
type OutgoingDataChannel struct {
	Label string

	dataChan *webrtc.DataChannel
}

func (c *OutgoingDataChannel) setup(p *PeerConnection) error {
	var err error
	c.dataChan, err = p.wr.CreateDataChannel(c.Label, &webrtc.DataChannelInit{
		Ordered: ptrOf(false),
	})
	if err != nil {
		return err
	}

	return nil
}

// Write writes data to the channel.
func (c *OutgoingDataChannel) Write(data []byte) {
	c.dataChan.Send(data) //nolint:errcheck
}
