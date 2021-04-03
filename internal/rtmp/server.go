package rtmp

import (
	"bufio"
	"net"

	"github.com/notedit/rtmp/format/rtmp"
)

// NewServerConn initializes a server-side connection.
func NewServerConn(nconn net.Conn) *Conn {
	// https://github.com/aler9/rtmp/blob/master/format/rtmp/server.go#L46
	rw := &bufio.ReadWriter{
		Reader: bufio.NewReaderSize(nconn, 4096),
		Writer: bufio.NewWriterSize(nconn, 4096),
	}
	c := rtmp.NewConn(rw)
	c.IsServer = true

	return &Conn{
		rconn: c,
		nconn: nconn,
	}
}

// ServerHandshake performs the handshake of a server-side connection.
func (c *Conn) ServerHandshake() error {
	return c.rconn.Prepare(rtmp.StageGotPublishOrPlayCommand, 0)
}
