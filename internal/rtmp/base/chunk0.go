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
	ChunkStreamID byte
	Timestamp     uint32
	Typ           byte
	StreamID      uint32
	BodyLen       uint32
	Body          []byte
}

// Read reads the chunk.
func (m *Chunk0) Read(r io.Reader, chunkMaxBodyLen int) error {
	header := make([]byte, 12)
	_, err := r.Read(header)
	if err != nil {
		return err
	}

	if header[0]>>6 != 0 {
		return fmt.Errorf("wrong chunk header type")
	}

	m.ChunkStreamID = header[0] & 0x3F
	m.Timestamp = uint32(header[3])<<16 | uint32(header[2])<<8 | uint32(header[1])
	m.BodyLen = uint32(header[4])<<16 | uint32(header[5])<<8 | uint32(header[6])
	m.Typ = header[7]
	m.StreamID = uint32(header[8])<<24 | uint32(header[9])<<16 | uint32(header[10])<<8 | uint32(header[11])

	chunkBodyLen := int(m.BodyLen)
	if chunkBodyLen > chunkMaxBodyLen {
		chunkBodyLen = chunkMaxBodyLen
	}

	m.Body = make([]byte, chunkBodyLen)
	_, err = r.Read(m.Body)
	return err
}

// Write writes the chunk.
func (m Chunk0) Write(w io.Writer) error {
	header := make([]byte, 12)
	header[0] = m.ChunkStreamID
	header[1] = byte(m.Timestamp >> 16)
	header[2] = byte(m.Timestamp >> 8)
	header[3] = byte(m.Timestamp)
	header[4] = byte(m.BodyLen >> 16)
	header[5] = byte(m.BodyLen >> 8)
	header[6] = byte(m.BodyLen)
	header[7] = m.Typ
	header[8] = byte(m.StreamID)
	_, err := w.Write(header)
	if err != nil {
		return err
	}

	_, err = w.Write(m.Body)
	return err
}
