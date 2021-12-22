package rtmp

import (
	"bufio"
	"net"

	"github.com/notedit/rtmp/format/rtmp"
)

// NewServerConn initializes a server-side connection.
func NewServerConn(nconn net.Conn) *Conn {
	// https://github.com/aler9/rtmp/blob/master/format/rtmp/server.go#L46
	c := rtmp.NewConn(&bufio.ReadWriter{
		Reader: bufio.NewReaderSize(nconn, readBufferSize),
		Writer: bufio.NewWriterSize(nconn, writeBufferSize),
	})
	c.IsServer = true

	return &Conn{
		rconn: c,
		nconn: nconn,
	}
}
