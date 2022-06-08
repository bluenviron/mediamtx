package chunk

import (
	"io"
)

// Chunk is a chunk.
type Chunk interface {
	Read(io.Reader, uint32) error
	Write() ([]byte, error)
}
