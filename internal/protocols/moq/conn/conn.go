// Package conn provides connections and streams for MoQ messages and subgroups.
package conn

import (
	"bufio"
	"context"
	"io"
	"net"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/controlmessage"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/rawconn"
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

// Conn is a wrapper around a raw connection that reads and writes MoQ primitives.
type Conn struct {
	Raw     rawconn.Conn
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
	_, ok := c.Raw.(*rawconn.QUIC)
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
	return &IncomingUniStream{stream: stream, reader: bufio.NewReader(stream), conn: c}, nil
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
