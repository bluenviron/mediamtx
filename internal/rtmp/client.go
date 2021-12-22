package rtmp

import (
	"bufio"
	"context"
	"net"
	"net/url"

	"github.com/notedit/rtmp/format/rtmp"
)

// DialContext connects to a server in reading mode.
func DialContext(ctx context.Context, address string) (*Conn, error) {
	// https://github.com/aler9/rtmp/blob/3be4a55359274dcd88762e72aa0a702e2d8ba2fd/format/rtmp/client.go#L74

	u, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	host := rtmp.UrlGetHost(u)

	var d net.Dialer
	nconn, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, err
	}

	rconn := rtmp.NewConn(&bufio.ReadWriter{
		Reader: bufio.NewReaderSize(nconn, readBufferSize),
		Writer: bufio.NewWriterSize(nconn, writeBufferSize),
	})
	rconn.URL = u

	return &Conn{
		rconn: rconn,
		nconn: nconn,
	}, nil
}
