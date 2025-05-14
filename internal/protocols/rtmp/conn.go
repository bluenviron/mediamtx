package rtmp

import (
	"io"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/bytecounter"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/message"
)

// Conn is implemented by Client and ServerConn.
type Conn interface {
	BytesReceived() uint64
	BytesSent() uint64
	Read() (message.Message, error)
	Write(msg message.Message) error
}

type dummyConn struct {
	rw io.ReadWriter

	bc  *bytecounter.ReadWriter
	mrw *message.ReadWriter
}

func (c *dummyConn) initialize() {
	c.bc = bytecounter.NewReadWriter(c.rw)
	c.mrw = message.NewReadWriter(c.bc, c.bc, false)
}

// BytesReceived returns the number of bytes received.
func (c *dummyConn) BytesReceived() uint64 {
	return c.bc.Reader.Count()
}

// BytesSent returns the number of bytes sent.
func (c *dummyConn) BytesSent() uint64 {
	return c.bc.Writer.Count()
}

func (c *dummyConn) Read() (message.Message, error) {
	return c.mrw.Read()
}

func (c *dummyConn) Write(msg message.Message) error {
	return c.mrw.Write(msg)
}
