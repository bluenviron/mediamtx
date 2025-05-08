package chunk

import (
	"io"
)

// Chunk2 is a type 2 chunk.
// Neither the stream ID nor the
// message length is included; this chunk has the same stream ID and
// message length as the preceding chunk.
type Chunk2 struct {
	ChunkStreamID  byte
	TimestampDelta uint32
	Body           []byte
}

// Read reads the chunk.
func (c *Chunk2) Read(r io.Reader, bodyLen uint32, _ bool) error {
	header := make([]byte, 4)
	_, err := io.ReadFull(r, header)
	if err != nil {
		return err
	}

	c.ChunkStreamID = header[0] & 0x3F
	c.TimestampDelta = uint32(header[1])<<16 | uint32(header[2])<<8 | uint32(header[3])

	if c.TimestampDelta >= 0xFFFFFF {
		_, err = io.ReadFull(r, header[:4])
		if err != nil {
			return err
		}

		c.TimestampDelta = uint32(header[0])<<24 | uint32(header[1])<<16 | uint32(header[2])<<8 | uint32(header[3])
	}

	c.Body = make([]byte, bodyLen)
	_, err = io.ReadFull(r, c.Body)
	return err
}

func (c Chunk2) marshalSize() int {
	n := 4 + len(c.Body)
	if c.TimestampDelta >= 0xFFFFFF {
		n += 4
	}
	return n
}

// Marshal writes the chunk.
func (c Chunk2) Marshal(_ bool) ([]byte, error) {
	buf := make([]byte, c.marshalSize())
	buf[0] = 2<<6 | c.ChunkStreamID

	if c.TimestampDelta >= 0xFFFFFF {
		buf[1] = 0xFF
		buf[2] = 0xFF
		buf[3] = 0xFF
		buf[4] = byte(c.TimestampDelta >> 24)
		buf[5] = byte(c.TimestampDelta >> 16)
		buf[6] = byte(c.TimestampDelta >> 8)
		buf[7] = byte(c.TimestampDelta)
		copy(buf[8:], c.Body)
	} else {
		buf[1] = byte(c.TimestampDelta >> 16)
		buf[2] = byte(c.TimestampDelta >> 8)
		buf[3] = byte(c.TimestampDelta)
		copy(buf[4:], c.Body)
	}

	return buf, nil
}
