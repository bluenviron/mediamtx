package rawconn

import (
	"context"
	"net"

	"github.com/quic-go/quic-go"
)

// QUIC is a connection built on top of QUIC.
type QUIC struct {
	Conn *quic.Conn
}

// CloseWithError implements the Conn interface.
func (c *QUIC) CloseWithError(code uint64, msg string) {
	_ = c.Conn.CloseWithError(quic.ApplicationErrorCode(code), msg)
}

// RemoteAddr implements the Conn interface.
func (c *QUIC) RemoteAddr() net.Addr {
	return c.Conn.RemoteAddr()
}

// OpenUniStreamSync implements the Conn interface.
func (c *QUIC) OpenUniStreamSync(ctx context.Context) (SendStream, error) {
	return c.Conn.OpenUniStreamSync(ctx)
}

// AcceptUniStream implements the Conn interface.
func (c *QUIC) AcceptUniStream(ctx context.Context) (ReceiveStream, error) {
	return c.Conn.AcceptUniStream(ctx)
}

// OpenStreamSync implements the Conn interface.
func (c *QUIC) OpenStreamSync(ctx context.Context) (BidiStream, error) {
	return c.Conn.OpenStreamSync(ctx)
}

// AcceptStream implements the Conn interface.
func (c *QUIC) AcceptStream(ctx context.Context) (BidiStream, error) {
	return c.Conn.AcceptStream(ctx)
}
