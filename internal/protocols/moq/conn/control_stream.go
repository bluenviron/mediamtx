package conn

import (
	"io"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/controlmessage"
)

// ControlStream reads and writes control messages on a bidirectional stream.
type ControlStream struct {
	stream io.ReadWriteCloser
	conn   *Conn
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
