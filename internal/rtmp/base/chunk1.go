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
	Body          []byte
}

// Write writes the chunk.
func (m Chunk1) Write(w io.Writer) error {
	header := make([]byte, 8)
	header[0] = 1<<6 | m.ChunkStreamID
	l := uint32(len(m.Body))
	header[4] = byte(l >> 16)
	header[5] = byte(l >> 8)
	header[6] = byte(l)
	header[7] = m.Typ
	_, err := w.Write(header)
	if err != nil {
		return err
	}

	_, err = w.Write(m.Body)
	return err
}
