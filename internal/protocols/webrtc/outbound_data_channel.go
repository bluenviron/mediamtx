package webrtc

import (
	"github.com/pion/webrtc/v4"
)

// OutboundDataChannel is an outgoing data channel.
type OutboundDataChannel struct {
	Label string

	dataChan *webrtc.DataChannel
}

func (c *OutboundDataChannel) setup(p *PeerConnection) error {
	var err error
	c.dataChan, err = p.wr.CreateDataChannel(c.Label, &webrtc.DataChannelInit{
		Ordered: new(false),
	})
	if err != nil {
		return err
	}

	return nil
}

// Write writes data to the channel, dropping data while it is connecting.
func (c *OutboundDataChannel) Write(data []byte) error {
	if c.dataChan.ReadyState() == webrtc.DataChannelStateConnecting {
		return nil
	}

	return c.dataChan.Send(data)
}
