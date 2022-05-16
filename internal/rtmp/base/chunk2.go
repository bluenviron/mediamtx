package base

import (
	"fmt"
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
func (c *Chunk2) Read(r io.Reader, chunkBodyLen int) error {
	header := make([]byte, 4)
	_, err := r.Read(header)
	if err != nil {
		return err
	}

	if header[0]>>6 != 2 {
		return fmt.Errorf("wrong chunk header type")
	}

	c.ChunkStreamID = header[0] & 0x3F
	c.TimestampDelta = uint32(header[3])<<16 | uint32(header[2])<<8 | uint32(header[1])

	c.Body = make([]byte, chunkBodyLen)
	_, err = r.Read(c.Body)
	return err
}

// Write writes the chunk.
func (c Chunk2) Write(w io.Writer) error {
	header := make([]byte, 4)
	header[0] = 1<<6 | c.ChunkStreamID
	header[1] = byte(c.TimestampDelta >> 16)
	header[2] = byte(c.TimestampDelta >> 8)
	header[3] = byte(c.TimestampDelta)
	_, err := w.Write(header)
	if err != nil {
		return err
	}

	_, err = w.Write(c.Body)
	return err
}
