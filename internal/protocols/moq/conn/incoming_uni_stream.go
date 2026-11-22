package conn

import (
	"bufio"
	"io"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/controlmessage"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
)

// IncomingUniStream reads control messages or subgroups.
type IncomingUniStream struct {
	stream *bufio.Reader
	conn   *Conn
}

// IsSubGroup reports whether the stream begins with a subgroup header.
func (s *IncomingUniStream) IsSubGroup() (bool, error) {
	first, err := s.stream.Peek(1)
	if err != nil {
		return false, err
	}
	return first[0]&0x90 == 0x10, nil
}

// ReadMessage reads and logs a control message.
func (s *IncomingUniStream) ReadMessage() (controlmessage.Message, error) {
	return readControlMessage(s.stream, s.conn)
}

// ReadSubGroup reads a subgroup.
func (s *IncomingUniStream) ReadSubGroup() (*subgroup.SubGroup, error) {
	var sg subgroup.SubGroup
	if err := sg.Read(s.stream); err != nil {
		return nil, err
	}
	return &sg, nil
}

// Drain discards any remaining bytes on the stream.
func (s *IncomingUniStream) Drain() error {
	_, err := io.Copy(io.Discard, s.stream)
	return err
}
