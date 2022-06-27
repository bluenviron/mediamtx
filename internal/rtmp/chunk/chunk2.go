package chunk

import (
	"io"
)

// Chunk2 is a type 2 chunk.
//  Neither the stream ID nor the
// message length is included; this chunk has the same stream ID and
// message length as the preceding chunk.
type Chunk2 struct {
	ChunkStreamID  byte
	TimestampDelta uint32
	Body           []byte
}

// Read reads the chunk.
func (c *Chunk2) Read(r io.Reader, chunkBodyLen uint32) error {
	header := make([]byte, 4)
	_, err := r.Read(header)
	if err != nil {
		return err
	}

	c.ChunkStreamID = header[0] & 0x3F
	c.TimestampDelta = uint32(header[1])<<16 | uint32(header[2])<<8 | uint32(header[3])

	c.Body = make([]byte, chunkBodyLen)
	_, err = r.Read(c.Body)
	return err
}

// Marshal writes the chunk.
func (c Chunk2) Marshal() ([]byte, error) {
	buf := make([]byte, 4+len(c.Body))
	buf[0] = 2<<6 | c.ChunkStreamID
	buf[1] = byte(c.TimestampDelta >> 16)
	buf[2] = byte(c.TimestampDelta >> 8)
	buf[3] = byte(c.TimestampDelta)
	copy(buf[4:], c.Body)
	return buf, nil
}
