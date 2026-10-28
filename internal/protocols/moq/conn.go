package moq

import (
	"bufio"
	"context"
	"io"
	"net"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/controlmessage"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/version"
)

func readControlMessage(r io.Reader, conn *Conn) (controlmessage.Message, error) {
	msg, err := controlmessage.Read(r, conn.Version)
	if err == nil {
		if conn.Server {
			conn.Log.Log(logger.Debug, "[c->s] %s", msg)
		} else {
			conn.Log.Log(logger.Debug, "[s->c] %s", msg)
		}
	}
	return msg, err
}

func writeControlMessage(w io.Writer, conn *Conn, msg controlmessage.Message) error {
	if conn.Server {
		conn.Log.Log(logger.Debug, "[s->c] %s", msg)
	} else {
		conn.Log.Log(logger.Debug, "[c->s] %s", msg)
	}
	_, err := w.Write(msg.Marshal(conn.Version))
	return err
}

// Conn is a wrapper around a rawConn that forces to read/write MoQ primitives.
type Conn struct {
	Raw     RawConn
	Server  bool
	Log     logger.Writer
	Version version.Version
}

// CloseWithError closes the underlying connection.
func (c *Conn) CloseWithError(code uint64, msg string) {
	c.Raw.CloseWithError(code, msg)
}

// RemoteAddr returns the underlying connection address.
func (c *Conn) RemoteAddr() net.Addr {
	return c.Raw.RemoteAddr()
}

// IsQUIC reports whether the underlying transport is native QUIC.
func (c *Conn) IsQUIC() bool {
	_, ok := c.Raw.(*RawConnQUIC)
	return ok
}

// OpenUniStreamSync opens a unidirectional stream for control messages or subgroups.
func (c *Conn) OpenUniStreamSync(ctx context.Context) (*OutgoingUniStream, error) {
	stream, err := c.Raw.OpenUniStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return &OutgoingUniStream{stream: stream, conn: c}, nil
}

// AcceptUniStream accepts an incoming unidirectional stream.
func (c *Conn) AcceptUniStream(ctx context.Context) (*IncomingUniStream, error) {
	stream, err := c.Raw.AcceptUniStream(ctx)
	if err != nil {
		return nil, err
	}
	return &IncomingUniStream{stream: bufio.NewReader(stream), conn: c}, nil
}

// OpenStreamSync opens a bidirectional control-message stream.
func (c *Conn) OpenStreamSync(ctx context.Context) (*ControlStream, error) {
	stream, err := c.Raw.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return &ControlStream{stream: stream, conn: c}, nil
}

// AcceptStream accepts a bidirectional control-message stream.
func (c *Conn) AcceptStream(ctx context.Context) (*ControlStream, error) {
	stream, err := c.Raw.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}
	return &ControlStream{stream: stream, conn: c}, nil
}

// OutgoingUniStream writes one or more control messages or subgroups.
type OutgoingUniStream struct {
	stream io.WriteCloser
	conn   *Conn
}

// WriteMessage writes and logs a control message.
func (s *OutgoingUniStream) WriteMessage(msg controlmessage.Message) error {
	return writeControlMessage(s.stream, s.conn, msg)
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
