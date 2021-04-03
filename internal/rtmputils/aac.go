package rtmputils

import (
	"time"

	"github.com/notedit/rtmp/av"
)

// WriteAACConfig writes an AAC config.
func (c *Conn) WriteAACConfig(config []byte) error {
	return c.WritePacket(av.Packet{
		Type: av.AACDecoderConfig,
		Data: config,
	})
}

// WriteAAC writes an AAC AU.
func (c *Conn) WriteAAC(au []byte, dts time.Duration) error {
	return c.WritePacket(av.Packet{
		Type: av.AAC,
		Data: au,
		Time: dts,
	})
}
