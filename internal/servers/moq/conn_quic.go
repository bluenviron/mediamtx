package moq //nolint:dupl

import (
	"context"
	"io"
	"net"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/quic-go/quic-go"
)

type connQUIC struct {
	conn *quic.Conn
}

func (c *connQUIC) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *connQUIC) OpenUniStreamSync(ctx context.Context) (io.WriteCloser, error) {
	return c.conn.OpenUniStreamSync(ctx)
}

func (c *connQUIC) AcceptUniStream(ctx context.Context) (io.Reader, error) {
	return c.conn.AcceptUniStream(ctx)
}

func (c *connQUIC) OpenStreamSync(ctx context.Context) (io.ReadWriteCloser, error) {
	return c.conn.OpenStreamSync(ctx)
}

func (c *connQUIC) AcceptStream(ctx context.Context) (io.ReadWriteCloser, error) {
	return c.conn.AcceptStream(ctx)
}

func (c *connQUIC) CloseWithError(code uint64, msg string) error {
	return c.conn.CloseWithError(quic.ApplicationErrorCode(code), msg)
}

func (*connQUIC) Transport() defs.APIMoQSessionTransport {
	return defs.APIMoQSessionTransportQUIC
}
