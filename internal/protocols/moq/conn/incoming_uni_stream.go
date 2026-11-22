package conn

import (
	"bufio"
	"io"
	"time"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/controlmessage"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/rawconn"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
)

// IncomingUniStream reads control messages or subgroups.
type IncomingUniStream struct {
	stream rawconn.ReceiveStream
	reader *bufio.Reader
	conn   *Conn
}

// SetReadDeadline sets the deadline for reads from the stream.
func (s *IncomingUniStream) SetReadDeadline(t time.Time) error {
	return s.stream.SetReadDeadline(t)
}

// IsSubGroup reports whether the stream begins with a subgroup header.
func (s *IncomingUniStream) IsSubGroup() (bool, error) {
	first, err := s.reader.Peek(1)
	if err != nil {
		return false, err
	}
	return first[0]&0x90 == 0x10, nil
}

// ReadMessage reads and logs a control message.
func (s *IncomingUniStream) ReadMessage() (controlmessage.Message, error) {
	return readControlMessage(s.reader, s.conn)
}

// ReadSubGroup reads a subgroup.
func (s *IncomingUniStream) ReadSubGroup() (*subgroup.SubGroup, error) {
	var sg subgroup.SubGroup
	if err := sg.Read(s.reader); err != nil {
		return nil, err
	}
	return &sg, nil
}

// Drain discards any remaining bytes on the stream.
func (s *IncomingUniStream) Drain() error {
	_, err := io.Copy(io.Discard, s.reader)
	return err
}
