// Package h264conf contains a H264 configuration parser.
package h264conf

import (
	"fmt"
)

// Conf is a RTMP H264 configuration.
type Conf struct {
	SPS []byte
	PPS []byte
}

// Unmarshal decodes a Conf from bytes.
func (c *Conf) Unmarshal(buf []byte) error {
	if len(buf) < 8 {
		return fmt.Errorf("invalid size 1")
	}

	pos := 5

	spsCount := buf[pos] & 0x1F
	pos++
	if spsCount != 1 {
		return fmt.Errorf("sps count != 1 is unsupported")
	}

	spsLen := int(uint16(buf[pos])<<8 | uint16(buf[pos+1]))
	pos += 2
	if (len(buf) - pos) < spsLen {
		return fmt.Errorf("invalid size 2")
	}

	c.SPS = buf[pos : pos+spsLen]
	pos += spsLen

	if (len(buf) - pos) < 3 {
		return fmt.Errorf("invalid size 3")
	}

	ppsCount := buf[pos]
	pos++
	if ppsCount != 1 {
		return fmt.Errorf("pps count != 1 is unsupported")
	}

	ppsLen := int(uint16(buf[pos])<<8 | uint16(buf[pos+1]))
	pos += 2
	if (len(buf) - pos) < ppsLen {
		return fmt.Errorf("invalid size")
	}

	c.PPS = buf[pos : pos+ppsLen]

	return nil
}

// Marshal encodes a Conf into bytes.
func (c Conf) Marshal() ([]byte, error) {
	spsLen := len(c.SPS)
	ppsLen := len(c.PPS)

	buf := make([]byte, 11+spsLen+ppsLen)

	buf[0] = 1
	buf[1] = c.SPS[1]
	buf[2] = c.SPS[2]
	buf[3] = c.SPS[3]
	buf[4] = 3 | 0xFC
	buf[5] = 1 | 0xE0
	pos := 6

	buf[pos] = byte(spsLen >> 8)
	buf[pos+1] = byte(spsLen)
	pos += 2

	copy(buf[pos:], c.SPS)
	pos += spsLen

	buf[pos] = 1
	pos++

	buf[pos] = byte(ppsLen >> 8)
	buf[pos+1] = byte(ppsLen)
	pos += 2

	copy(buf[pos:], c.PPS)

	return buf, nil
}
