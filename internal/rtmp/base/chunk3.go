package base

import (
	"fmt"
	"io"
)

// Chunk3 is a type 3 chunk.
// Type 3 chunks have no message header. The stream ID, message length
// and timestamp delta fields are not present; chunks of this type take
// values from the preceding chunk for the same Chunk Stream ID. When a
// single message is split into chunks, all chunks of a message except
// the first one SHOULD use this type.
type Chunk3 struct {
	ChunkStreamID byte
	Body          []byte
}

// Read reads the chunk.
func (c *Chunk3) Read(r io.Reader, chunkBodyLen int) error {
	header := make([]byte, 1)
	_, err := r.Read(header)
	if err != nil {
		return err
	}

	if header[0]>>6 != 2 {
		return fmt.Errorf("wrong chunk header type")
	}

	c.ChunkStreamID = header[0] & 0x3F

	c.Body = make([]byte, chunkBodyLen)
	_, err = r.Read(c.Body)
	return err
}

// Write writes the chunk.
func (c Chunk3) Write(w io.Writer) error {
	header := make([]byte, 1)
	header[0] = 3<<6 | c.ChunkStreamID
	_, err := w.Write(header)
	if err != nil {
		return err
	}

	_, err = w.Write(c.Body)
	return err
}
