package conn

import (
	"time"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/controlmessage"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/rawconn"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
)

// OutgoingUniStream writes one or more control messages or subgroups.
type OutgoingUniStream struct {
	stream rawconn.SendStream
	conn   *Conn
}

// WriteMessage writes and logs a control message.
func (s *OutgoingUniStream) WriteMessage(msg controlmessage.Message) error {
	return writeControlMessage(s.stream, s.conn, msg)
}

// SetWriteDeadline sets the deadline for writes to the stream.
func (s *OutgoingUniStream) SetWriteDeadline(t time.Time) error {
	return s.stream.SetWriteDeadline(t)
}

// WriteSubGroup writes a subgroup.
func (s *OutgoingUniStream) WriteSubGroup(sg *subgroup.SubGroup) error {
	_, err := s.stream.Write(sg.Marshal())
	return err
}

// Close closes the stream.
func (s *OutgoingUniStream) Close() error {
	return s.stream.Close()
}
