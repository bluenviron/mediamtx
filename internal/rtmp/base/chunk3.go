package base

import (
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

// Write writes the chunk.
func (m Chunk3) Write(w io.Writer) error {
	header := make([]byte, 1)
	header[0] = 3<<6 | m.ChunkStreamID
	_, err := w.Write(header)
	if err != nil {
		return err
	}

	_, err = w.Write(m.Body)
	return err
}
