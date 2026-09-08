package moq

import (
	"context"
	"io"
	"net"

	"github.com/quic-go/quic-go"
)

// ConnQUIC is a connection built on top of QUIC.
type ConnQUIC struct {
	Conn *quic.Conn
}

// CloseWithError implements the Conn interface.
func (c *ConnQUIC) CloseWithError(code uint64, msg string) {
	_ = c.Conn.CloseWithError(quic.ApplicationErrorCode(code), msg)
}

// RemoteAddr implements the Conn interface.
func (c *ConnQUIC) RemoteAddr() net.Addr {
	return c.Conn.RemoteAddr()
}

// OpenUniStreamSync implements the Conn interface.
func (c *ConnQUIC) OpenUniStreamSync(ctx context.Context) (io.WriteCloser, error) {
	return c.Conn.OpenUniStreamSync(ctx)
}

// AcceptUniStream implements the Conn interface.
func (c *ConnQUIC) AcceptUniStream(ctx context.Context) (io.Reader, error) {
	return c.Conn.AcceptUniStream(ctx)
}

// OpenStreamSync implements the Conn interface.
func (c *ConnQUIC) OpenStreamSync(ctx context.Context) (io.ReadWriteCloser, error) {
	return c.Conn.OpenStreamSync(ctx)
}

// AcceptStream implements the Conn interface.
func (c *ConnQUIC) AcceptStream(ctx context.Context) (io.ReadWriteCloser, error) {
	return c.Conn.AcceptStream(ctx)
}
