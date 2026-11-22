// Package rawconn adapts QUIC and WebTransport connections for MoQ.
package rawconn

import (
	"context"
	"io"
	"net"
	"time"
)

// SendStream is the send side of a QUIC or WebTransport stream.
type SendStream interface {
	io.WriteCloser
	SetWriteDeadline(time.Time) error
}

// ReceiveStream is the receive side of a QUIC or WebTransport stream.
type ReceiveStream interface {
	io.Reader
	SetReadDeadline(time.Time) error
}

// BidiStream is a bidirectional QUIC or WebTransport stream.
type BidiStream interface {
	SendStream
	ReceiveStream
}

// Conn is a connection built on top of either QUIC or WebTransport.
type Conn interface {
	CloseWithError(code uint64, msg string)
	RemoteAddr() net.Addr
	OpenUniStreamSync(ctx context.Context) (SendStream, error)
	AcceptUniStream(ctx context.Context) (ReceiveStream, error)
	OpenStreamSync(ctx context.Context) (BidiStream, error)
	AcceptStream(ctx context.Context) (BidiStream, error)
}
