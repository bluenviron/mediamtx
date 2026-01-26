// Package unix contains utilities to work with Unix sockets.
package unix

import (
	"fmt"
	"net"
	"net/url"
	"os"

	"github.com/bluenviron/gortsplib/v5/pkg/readbuffer"
)

// Listen creates a Unix listener on the given URL.
func Listen(u *url.URL, udpReadBufferSize int) (net.Conn, error) {
	var pa string
	if u.Path != "" {
		pa = u.Path
	} else {
		pa = u.Host
	}

	if pa == "" {
		return nil, fmt.Errorf("invalid unix path")
	}

	os.Remove(pa)

	addr, err := net.ResolveUnixAddr("unixgram", pa)
	if err != nil {
		panic(err)
	}

	socket, err := net.ListenUnixgram("unixgram", addr)
	if err != nil {
		return nil, err
	}

	if udpReadBufferSize != 0 {
		err = readbuffer.SetReadBuffer(socket, udpReadBufferSize)
		if err != nil {
			socket.Close() //nolint:errcheck
			return nil, err
		}
	}

	return socket, nil
}
