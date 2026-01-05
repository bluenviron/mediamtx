// Package unixgram contains utilities to work with Unix sockets.
package unixgram

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"syscall"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/readbuffer"
)

type packetConn interface {
	net.PacketConn
	SetReadBuffer(bytes int) error
	SyscallConn() (syscall.RawConn, error)
}

type unixgramConn struct {
	pc net.PacketConn
}

func (r *unixgramConn) Close() error {
	return r.pc.Close()
}

func (r *unixgramConn) Read(p []byte) (int, error) {
	n, _, err := r.pc.ReadFrom(p)
	return n, err
}

func (r *unixgramConn) Write(_ []byte) (int, error) {
	panic("unimplemented")
}

func (r *unixgramConn) LocalAddr() net.Addr {
	panic("unimplemented")
}

func (r *unixgramConn) RemoteAddr() net.Addr {
	panic("unimplemented")
}

func (r *unixgramConn) SetDeadline(_ time.Time) error {
	panic("unimplemented")
}

func (r *unixgramConn) SetReadDeadline(t time.Time) error {
	return r.pc.SetReadDeadline(t)
}

func (r *unixgramConn) SetWriteDeadline(_ time.Time) error {
	panic("unimplemented")
}

// CreateConn creates a Unix socket connection.
func CreateConn(u *url.URL, udpReadBufferSize int) (net.Conn, error) {
	var pa string
	if u.Path != "" {
		pa = u.Path
	} else {
		pa = u.Host
	}

	if pa == "" {
		return nil, fmt.Errorf("invalid unixgram path")
	}

	os.Remove(pa)

	var pc packetConn
	var tmp net.PacketConn
	tmp, err := net.ListenPacket("unixgram", pa)
	if err != nil {
		return nil, err
	}
	pc = tmp.(*net.UnixConn)

	if udpReadBufferSize != 0 {
		err = readbuffer.SetReadBuffer(pc, udpReadBufferSize)
		if err != nil {
			pc.Close()
			return nil, err
		}
	}

	return &unixgramConn{pc: pc}, nil
}
