package chunk

import (
	"io"
)

// Chunk0 is a type 0 chunk.
// This type MUST be used at
// the start of a chunk stream, and whenever the stream timestamp goes
// backward (e.g., because of a backward seek).
type Chunk0 struct {
	ChunkStreamID   byte
	Timestamp       uint32
	BodyLen         uint32
	Type            uint8
	MessageStreamID uint32
	Body            []byte
}

// Read reads the chunk.
func (c *Chunk0) Read(r io.Reader, maxBodyLen uint32, _ bool) error {
	header := make([]byte, 12)
	_, err := io.ReadFull(r, header)
	if err != nil {
		return err
	}

	c.ChunkStreamID = header[0] & 0x3F
	c.Timestamp = uint32(header[1])<<16 | uint32(header[2])<<8 | uint32(header[3])
	c.BodyLen = uint32(header[4])<<16 | uint32(header[5])<<8 | uint32(header[6])
	c.Type = header[7]
	c.MessageStreamID = uint32(header[8])<<24 | uint32(header[9])<<16 | uint32(header[10])<<8 | uint32(header[11])

	if c.Timestamp >= 0xFFFFFF {
		_, err = io.ReadFull(r, header[:4])
		if err != nil {
			return err
		}

		c.Timestamp = uint32(header[0])<<24 | uint32(header[1])<<16 | uint32(header[2])<<8 | uint32(header[3])
	}

	chunkBodyLen := c.BodyLen
	if chunkBodyLen > maxBodyLen {
		chunkBodyLen = maxBodyLen
	}

	c.Body = make([]byte, chunkBodyLen)
	_, err = io.ReadFull(r, c.Body)
	return err
}

func (c Chunk0) marshalSize() int {
	n := 12 + len(c.Body)
	if c.Timestamp >= 0xFFFFFF {
		n += 4
	}
	return n
}

// Marshal writes the chunk.
func (c Chunk0) Marshal(_ bool) ([]byte, error) {
	buf := make([]byte, c.marshalSize())
	buf[0] = c.ChunkStreamID

	if c.Timestamp >= 0xFFFFFF {
		buf[1] = 0xFF
		buf[2] = 0xFF
		buf[3] = 0xFF
		buf[4] = byte(c.BodyLen >> 16)
		buf[5] = byte(c.BodyLen >> 8)
		buf[6] = byte(c.BodyLen)
		buf[7] = c.Type
		buf[8] = byte(c.MessageStreamID >> 24)
		buf[9] = byte(c.MessageStreamID >> 16)
		buf[10] = byte(c.MessageStreamID >> 8)
		buf[11] = byte(c.MessageStreamID)
		buf[12] = byte(c.Timestamp >> 24)
		buf[13] = byte(c.Timestamp >> 16)
		buf[14] = byte(c.Timestamp >> 8)
		buf[15] = byte(c.Timestamp)
		copy(buf[16:], c.Body)
	} else {
		buf[1] = byte(c.Timestamp >> 16)
		buf[2] = byte(c.Timestamp >> 8)
		buf[3] = byte(c.Timestamp)
		buf[4] = byte(c.BodyLen >> 16)
		buf[5] = byte(c.BodyLen >> 8)
		buf[6] = byte(c.BodyLen)
		buf[7] = c.Type
		buf[8] = byte(c.MessageStreamID >> 24)
		buf[9] = byte(c.MessageStreamID >> 16)
		buf[10] = byte(c.MessageStreamID >> 8)
		buf[11] = byte(c.MessageStreamID)
		copy(buf[12:], c.Body)
	}

	return buf, nil
}
