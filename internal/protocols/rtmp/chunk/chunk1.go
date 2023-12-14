package chunk

import (
	"io"
)

// Chunk1 is a type 1 chunk.
// The message stream ID is not
// included; this chunk takes the same stream ID as the preceding chunk.
// Streams with variable-sized messages (for example, many video
// formats) SHOULD use this format for the first chunk of each new
// message after the first.
type Chunk1 struct {
	ChunkStreamID  byte
	TimestampDelta uint32
	BodyLen        uint32
	Type           uint8
	Body           []byte
}

// Read reads the chunk.
func (c *Chunk1) Read(r io.Reader, maxBodyLen uint32, _ bool) error {
	header := make([]byte, 8)
	_, err := io.ReadFull(r, header)
	if err != nil {
		return err
	}

	c.ChunkStreamID = header[0] & 0x3F
	c.TimestampDelta = uint32(header[1])<<16 | uint32(header[2])<<8 | uint32(header[3])
	c.BodyLen = uint32(header[4])<<16 | uint32(header[5])<<8 | uint32(header[6])
	c.Type = header[7]

	if c.TimestampDelta >= 0xFFFFFF {
		_, err = io.ReadFull(r, header[:4])
		if err != nil {
			return err
		}

		c.TimestampDelta = uint32(header[0])<<24 | uint32(header[1])<<16 | uint32(header[2])<<8 | uint32(header[3])
	}

	chunkBodyLen := (c.BodyLen)
	if chunkBodyLen > maxBodyLen {
		chunkBodyLen = maxBodyLen
	}

	c.Body = make([]byte, chunkBodyLen)
	_, err = io.ReadFull(r, c.Body)
	return err
}

func (c Chunk1) marshalSize() int {
	n := 8 + len(c.Body)
	if c.TimestampDelta >= 0xFFFFFF {
		n += 4
	}
	return n
}

// Marshal writes the chunk.
func (c Chunk1) Marshal(_ bool) ([]byte, error) {
	buf := make([]byte, c.marshalSize())
	buf[0] = 1<<6 | c.ChunkStreamID

	if c.TimestampDelta >= 0xFFFFFF {
		buf[1] = 0xFF
		buf[2] = 0xFF
		buf[3] = 0xFF
		buf[4] = byte(c.BodyLen >> 16)
		buf[5] = byte(c.BodyLen >> 8)
		buf[6] = byte(c.BodyLen)
		buf[7] = c.Type
		buf[8] = byte(c.TimestampDelta >> 24)
		buf[9] = byte(c.TimestampDelta >> 16)
		buf[10] = byte(c.TimestampDelta >> 8)
		buf[11] = byte(c.TimestampDelta)
		copy(buf[12:], c.Body)
	} else {
		buf[1] = byte(c.TimestampDelta >> 16)
		buf[2] = byte(c.TimestampDelta >> 8)
		buf[3] = byte(c.TimestampDelta)
		buf[4] = byte(c.BodyLen >> 16)
		buf[5] = byte(c.BodyLen >> 8)
		buf[6] = byte(c.BodyLen)
		buf[7] = c.Type
		copy(buf[8:], c.Body)
	}

	return buf, nil
}
