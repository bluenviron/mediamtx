package base

import (
	"fmt"
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
func (c *Chunk1) Read(r io.Reader, chunkMaxBodyLen int) error {
	header := make([]byte, 8)
	_, err := r.Read(header)
	if err != nil {
		return err
	}

	if header[0]>>6 != 1 {
		return fmt.Errorf("wrong chunk header type")
	}

	c.ChunkStreamID = header[0] & 0x3F
	c.TimestampDelta = uint32(header[3])<<16 | uint32(header[2])<<8 | uint32(header[1])
	c.BodyLen = uint32(header[4])<<16 | uint32(header[5])<<8 | uint32(header[6])
	c.Type = MessageType(header[7])

	chunkBodyLen := int(c.BodyLen)
	if chunkBodyLen > chunkMaxBodyLen {
		chunkBodyLen = chunkMaxBodyLen
	}

	c.Body = make([]byte, chunkBodyLen)
	_, err = r.Read(c.Body)
	return err
}

// Write writes the chunk.
func (c Chunk1) Write(w io.Writer) error {
	header := make([]byte, 8)
	header[0] = 1<<6 | c.ChunkStreamID
	header[1] = byte(c.TimestampDelta >> 16)
	header[2] = byte(c.TimestampDelta >> 8)
	header[3] = byte(c.TimestampDelta)
	header[4] = byte(c.BodyLen >> 16)
	header[5] = byte(c.BodyLen >> 8)
	header[6] = byte(c.BodyLen)
	header[7] = byte(c.Type)
	_, err := w.Write(header)
	if err != nil {
		return err
	}

	_, err = w.Write(c.Body)
	return err
}
