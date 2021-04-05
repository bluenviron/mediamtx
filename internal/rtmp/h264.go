package rtmp

import (
	"time"

	"github.com/notedit/rtmp/av"

	"github.com/aler9/rtsp-simple-server/internal/h264"
)

// WriteH264 writes H264 NALUs.
func (c *Conn) WriteH264(nalus [][]byte, dts time.Duration) error {
	data, err := h264.EncodeAVCC(nalus)
	if err != nil {
		return err
	}

	return c.WritePacket(av.Packet{
		Type: av.H264,
		Data: data,
		Time: dts,
	})
}
