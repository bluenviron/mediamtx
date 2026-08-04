package moq

import (
	"context"
	"io"
	"net"

	"github.com/bluenviron/mediamtx/internal/defs"
)

type conn interface {
	RemoteAddr() net.Addr
	OpenUniStreamSync(ctx context.Context) (io.WriteCloser, error)
	AcceptUniStream(ctx context.Context) (io.Reader, error)
	OpenStreamSync(ctx context.Context) (io.ReadWriteCloser, error)
	AcceptStream(ctx context.Context) (io.ReadWriteCloser, error)
	CloseWithError(code uint64, msg string) error
	Transport() defs.APIMoQSessionTransport
}
