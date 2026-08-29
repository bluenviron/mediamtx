package moq

import (
	"context"
	"io"
	"net"
)

// Conn is a connection built on top of either QUIC or WebTransport.
type Conn interface {
	CloseWithError(code uint64, msg string)
	RemoteAddr() net.Addr
	OpenUniStreamSync(ctx context.Context) (io.WriteCloser, error)
	AcceptUniStream(ctx context.Context) (io.Reader, error)
	OpenStreamSync(ctx context.Context) (io.ReadWriteCloser, error)
	AcceptStream(ctx context.Context) (io.ReadWriteCloser, error)
}
