package moq

import (
	"context"
	"io"
	"net"

	"github.com/quic-go/webtransport-go"
)

// ConnWebTransport is a connection built on top of WebTransport.
type ConnWebTransport struct {
	Session      *webtransport.Session
	Transport    *webtransport.Transport
	ResponseBody io.Closer
}

// CloseWithError implements the Conn interface.
func (c *ConnWebTransport) CloseWithError(code uint64, msg string) {
	c.Session.CloseWithError(webtransport.SessionErrorCode(code), msg) //nolint:errcheck

	if c.ResponseBody != nil {
		c.ResponseBody.Close() //nolint:errcheck
	}

	if c.Transport != nil {
		c.Transport.Close() //nolint:errcheck
	}
}

// RemoteAddr implements the Conn interface.
func (c *ConnWebTransport) RemoteAddr() net.Addr {
	return c.Session.RemoteAddr()
}

// OpenUniStreamSync implements the Conn interface.
func (c *ConnWebTransport) OpenUniStreamSync(ctx context.Context) (io.WriteCloser, error) {
	return c.Session.OpenUniStreamSync(ctx)
}

// AcceptUniStream implements the Conn interface.
func (c *ConnWebTransport) AcceptUniStream(ctx context.Context) (io.Reader, error) {
	return c.Session.AcceptUniStream(ctx)
}

// OpenStreamSync implements the Conn interface.
func (c *ConnWebTransport) OpenStreamSync(ctx context.Context) (io.ReadWriteCloser, error) {
	return c.Session.OpenStreamSync(ctx)
}

// AcceptStream implements the Conn interface.
func (c *ConnWebTransport) AcceptStream(ctx context.Context) (io.ReadWriteCloser, error) {
	return c.Session.AcceptStream(ctx)
}
