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
	Type           MessageType
	BodyLen        uint32
	Body           []byte
}

// Read reads the chunk.
func (c *Chunk1) Read(r io.Reader, chunkMaxBodyLen uint32) error {
	header := make([]byte, 8)
	_, err := r.Read(header)
	if err != nil {
		return err
	}

	c.ChunkStreamID = header[0] & 0x3F
	c.TimestampDelta = uint32(header[1])<<16 | uint32(header[2])<<8 | uint32(header[3])
	c.BodyLen = uint32(header[4])<<16 | uint32(header[5])<<8 | uint32(header[6])
	c.Type = MessageType(header[7])

	chunkBodyLen := (c.BodyLen)
	if chunkBodyLen > chunkMaxBodyLen {
		chunkBodyLen = chunkMaxBodyLen
	}

	c.Body = make([]byte, chunkBodyLen)
	_, err = r.Read(c.Body)
	return err
}

// Marshal writes the chunk.
func (c Chunk1) Marshal() ([]byte, error) {
	buf := make([]byte, 8+len(c.Body))
	buf[0] = 1<<6 | c.ChunkStreamID
	buf[1] = byte(c.TimestampDelta >> 16)
	buf[2] = byte(c.TimestampDelta >> 8)
	buf[3] = byte(c.TimestampDelta)
	buf[4] = byte(c.BodyLen >> 16)
	buf[5] = byte(c.BodyLen >> 8)
	buf[6] = byte(c.BodyLen)
	buf[7] = byte(c.Type)
	copy(buf[8:], c.Body)
	return buf, nil
}
