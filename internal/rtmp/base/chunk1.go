package base

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
	ChunkStreamID byte
	Typ           byte
	BodyLen       uint32
	Body          []byte
}

// Write writes the chunk.
func (c Chunk1) Write(w io.Writer) error {
	header := make([]byte, 8)
	header[0] = 1<<6 | c.ChunkStreamID
	header[4] = byte(c.BodyLen >> 16)
	header[5] = byte(c.BodyLen >> 8)
	header[6] = byte(c.BodyLen)
	header[7] = c.Typ
	_, err := w.Write(header)
	if err != nil {
		return err
	}

	_, err = w.Write(c.Body)
	return err
}
