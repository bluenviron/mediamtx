package moq //nolint:dupl

import (
	"context"
	"io"
	"net"

	"github.com/quic-go/webtransport-go"

	"github.com/bluenviron/mediamtx/internal/defs"
)

type connWebTransport struct {
	session *webtransport.Session
}

func (c *connWebTransport) RemoteAddr() net.Addr {
	return c.session.RemoteAddr()
}

func (c *connWebTransport) OpenUniStreamSync(ctx context.Context) (io.WriteCloser, error) {
	return c.session.OpenUniStreamSync(ctx)
}

func (c *connWebTransport) AcceptUniStream(ctx context.Context) (io.Reader, error) {
	return c.session.AcceptUniStream(ctx)
}

func (c *connWebTransport) OpenStreamSync(ctx context.Context) (io.ReadWriteCloser, error) {
	return c.session.OpenStreamSync(ctx)
}

func (c *connWebTransport) AcceptStream(ctx context.Context) (io.ReadWriteCloser, error) {
	return c.session.AcceptStream(ctx)
}

func (c *connWebTransport) CloseWithError(code uint64, msg string) error {
	return c.session.CloseWithError(webtransport.SessionErrorCode(code), msg)
}

func (*connWebTransport) Transport() defs.APIMoQSessionTransport {
	return defs.APIMoQSessionTransportWebTransport
}
