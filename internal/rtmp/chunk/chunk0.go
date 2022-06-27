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
	Type            MessageType
	MessageStreamID uint32
	BodyLen         uint32
	Body            []byte
}

// Read reads the chunk.
func (c *Chunk0) Read(r io.Reader, chunkMaxBodyLen uint32) error {
	header := make([]byte, 12)
	_, err := r.Read(header)
	if err != nil {
		return err
	}

	c.ChunkStreamID = header[0] & 0x3F
	c.Timestamp = uint32(header[1])<<16 | uint32(header[2])<<8 | uint32(header[3])
	c.BodyLen = uint32(header[4])<<16 | uint32(header[5])<<8 | uint32(header[6])
	c.Type = MessageType(header[7])
	c.MessageStreamID = uint32(header[8])<<24 | uint32(header[9])<<16 | uint32(header[10])<<8 | uint32(header[11])

	chunkBodyLen := c.BodyLen
	if chunkBodyLen > chunkMaxBodyLen {
		chunkBodyLen = chunkMaxBodyLen
	}

	c.Body = make([]byte, chunkBodyLen)
	_, err = r.Read(c.Body)
	return err
}

// Marshal writes the chunk.
func (c Chunk0) Marshal() ([]byte, error) {
	buf := make([]byte, 12+len(c.Body))
	buf[0] = c.ChunkStreamID
	buf[1] = byte(c.Timestamp >> 16)
	buf[2] = byte(c.Timestamp >> 8)
	buf[3] = byte(c.Timestamp)
	buf[4] = byte(c.BodyLen >> 16)
	buf[5] = byte(c.BodyLen >> 8)
	buf[6] = byte(c.BodyLen)
	buf[7] = byte(c.Type)
	buf[8] = byte(c.MessageStreamID >> 24)
	buf[9] = byte(c.MessageStreamID >> 16)
	buf[10] = byte(c.MessageStreamID >> 8)
	buf[11] = byte(c.MessageStreamID)
	copy(buf[12:], c.Body)
	return buf, nil
}
