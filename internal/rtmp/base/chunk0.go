package base

import (
	"fmt"
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
func (c *Chunk0) Read(r io.Reader, chunkMaxBodyLen int) error {
	header := make([]byte, 12)
	_, err := r.Read(header)
	if err != nil {
		return err
	}

	if header[0]>>6 != 0 {
		return fmt.Errorf("wrong chunk header type")
	}

	c.ChunkStreamID = header[0] & 0x3F
	c.Timestamp = uint32(header[3])<<16 | uint32(header[2])<<8 | uint32(header[1])
	c.BodyLen = uint32(header[4])<<16 | uint32(header[5])<<8 | uint32(header[6])
	c.Type = MessageType(header[7])
	c.MessageStreamID = uint32(header[8])<<24 | uint32(header[9])<<16 | uint32(header[10])<<8 | uint32(header[11])

	chunkBodyLen := int(c.BodyLen)
	if chunkBodyLen > chunkMaxBodyLen {
		chunkBodyLen = chunkMaxBodyLen
	}

	c.Body = make([]byte, chunkBodyLen)
	_, err = r.Read(c.Body)
	return err
}

// Write writes the chunk.
func (c Chunk0) Write(w io.Writer) error {
	header := make([]byte, 12)
	header[0] = c.ChunkStreamID
	header[1] = byte(c.Timestamp >> 16)
	header[2] = byte(c.Timestamp >> 8)
	header[3] = byte(c.Timestamp)
	header[4] = byte(c.BodyLen >> 16)
	header[5] = byte(c.BodyLen >> 8)
	header[6] = byte(c.BodyLen)
	header[7] = byte(c.Type)
	header[8] = byte(c.MessageStreamID)
	_, err := w.Write(header)
	if err != nil {
		return err
	}

	_, err = w.Write(c.Body)
	return err
}
