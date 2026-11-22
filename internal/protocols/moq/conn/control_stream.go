package conn

import (
	"io"
	"time"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/controlmessage"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/rawconn"
)

// ControlStream reads and writes control messages on a bidirectional stream.
type ControlStream struct {
	stream rawconn.BidiStream
	conn   *Conn
}

// SetReadDeadline sets the deadline for reads from the stream.
func (s *ControlStream) SetReadDeadline(t time.Time) error {
	return s.stream.SetReadDeadline(t)
}

// SetWriteDeadline sets the deadline for writes to the stream.
func (s *ControlStream) SetWriteDeadline(t time.Time) error {
	return s.stream.SetWriteDeadline(t)
}

// ReadMessage reads and logs a control message.
func (s *ControlStream) ReadMessage() (controlmessage.Message, error) {
	return readControlMessage(s.stream, s.conn)
}

// WriteMessage writes and logs a control message.
func (s *ControlStream) WriteMessage(msg controlmessage.Message) error {
	return writeControlMessage(s.stream, s.conn, msg)
}

// Close closes the stream.
func (s *ControlStream) Close() error {
	return s.stream.Close()
}

// Drain discards any remaining bytes on the stream.
func (s *ControlStream) Drain() error {
	_, err := io.Copy(io.Discard, s.stream)
	return err
}
