package chunk

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

// Read reads the chunk.
func (c *Chunk3) Read(r io.Reader, bodyLen uint32, hasExtendedTimestamp bool) error {
	header := make([]byte, 4)
	_, err := io.ReadFull(r, header[:1])
	if err != nil {
		return err
	}

	c.ChunkStreamID = header[0] & 0x3F

	if hasExtendedTimestamp {
		_, err = io.ReadFull(r, header[:4])
		if err != nil {
			return err
		}
	}

	c.Body = make([]byte, bodyLen)
	_, err = io.ReadFull(r, c.Body)
	return err
}

func (c Chunk3) marshalSize(hasExtendedTimestamp bool) int {
	n := 1 + len(c.Body)
	if hasExtendedTimestamp {
		n += 4
	}
	return n
}

// Marshal writes the chunk.
func (c Chunk3) Marshal(hasExtendedTimestamp bool) ([]byte, error) {
	buf := make([]byte, c.marshalSize(hasExtendedTimestamp))
	buf[0] = 3<<6 | c.ChunkStreamID

	if hasExtendedTimestamp {
		copy(buf[5:], c.Body)
	} else {
		copy(buf[1:], c.Body)
	}

	return buf, nil
}
