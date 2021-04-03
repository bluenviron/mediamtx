package rtmputils

import (
	"time"

	"github.com/notedit/rtmp/av"
	rh264 "github.com/notedit/rtmp/codec/h264"

	"github.com/aler9/rtsp-simple-server/internal/h264"
)

// WriteH264Config writes a H264 config.
func (c *Conn) WriteH264Config(sps []byte, pps []byte) error {
	codec := rh264.Codec{
		SPS: map[int][]byte{
			0: sps,
		},
		PPS: map[int][]byte{
			0: pps,
		},
	}
	b := make([]byte, 128)
	var n int
	codec.ToConfig(b, &n)
	b = b[:n]

	return c.WritePacket(av.Packet{
		Type: av.H264DecoderConfig,
		Data: b,
	})
}

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
