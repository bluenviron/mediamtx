package rtmp

import (
	"time"

	"github.com/notedit/rtmp/av"
)

// WriteAAC writes an AAC AU.
func (c *Conn) WriteAAC(au []byte, dts time.Duration) error {
	return c.WritePacket(av.Packet{
		Type: av.AAC,
		Data: au,
		Time: dts,
	})
}
