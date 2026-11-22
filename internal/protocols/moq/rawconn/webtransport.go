package rawconn

import (
	"context"
	"io"
	"net"

	"github.com/quic-go/webtransport-go"
)

// WebTransport is a connection built on top of WebTransport.
type WebTransport struct {
	Session      *webtransport.Session
	Transport    *webtransport.Transport
	ResponseBody io.Closer
}

// CloseWithError implements the Conn interface.
func (c *WebTransport) CloseWithError(code uint64, msg string) {
	c.Session.CloseWithError(webtransport.SessionErrorCode(code), msg) //nolint:errcheck

	if c.ResponseBody != nil {
		c.ResponseBody.Close() //nolint:errcheck
	}

	if c.Transport != nil {
		c.Transport.Close() //nolint:errcheck
	}
}

// RemoteAddr implements the Conn interface.
func (c *WebTransport) RemoteAddr() net.Addr {
	return c.Session.RemoteAddr()
}

// OpenUniStreamSync implements the Conn interface.
func (c *WebTransport) OpenUniStreamSync(ctx context.Context) (SendStream, error) {
	return c.Session.OpenUniStreamSync(ctx)
}

// AcceptUniStream implements the Conn interface.
func (c *WebTransport) AcceptUniStream(ctx context.Context) (ReceiveStream, error) {
	return c.Session.AcceptUniStream(ctx)
}

// OpenStreamSync implements the Conn interface.
func (c *WebTransport) OpenStreamSync(ctx context.Context) (BidiStream, error) {
	return c.Session.OpenStreamSync(ctx)
}

// AcceptStream implements the Conn interface.
func (c *WebTransport) AcceptStream(ctx context.Context) (BidiStream, error) {
	return c.Session.AcceptStream(ctx)
}
