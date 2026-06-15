// Package unixgram contains utilities to work with Unix datagram sockets.
package unixgram

import (
	"fmt"
	"net"
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

// Listener is a listener on a Unix datagram socket.
//
// RTP is packet-oriented and relies on message boundaries, which a datagram
// socket ("unixgram") preserves.
type Listener struct {
	Path              string
	UDPReadBufferSize int
	ListenPacket      func(network string, address string) (net.PacketConn, error)

	pc packetConn
}

// Initialize initializes the listener.
func (l *Listener) Initialize() error {
	if l.Path == "" {
		return fmt.Errorf("invalid unix path")
	}
	if l.ListenPacket == nil {
		l.ListenPacket = net.ListenPacket
	}

	os.Remove(l.Path)

	tmp, err := l.ListenPacket("unixgram", l.Path)
	if err != nil {
		return err
	}
	l.pc = tmp.(packetConn)

	if l.UDPReadBufferSize != 0 {
		err = readbuffer.SetReadBuffer(l.pc, l.UDPReadBufferSize)
		if err != nil {
			l.pc.Close() //nolint:errcheck
			return err
		}
	}

	return nil
}

// Close closes the listener.
func (l *Listener) Close() error {
	err := l.pc.Close()
	// Datagram sockets are not unlinked automatically on close.
	os.Remove(l.Path) //nolint:errcheck
	return err
}

// Read implements net.Conn.
func (l *Listener) Read(p []byte) (int, error) {
	n, _, err := l.pc.ReadFrom(p)
	return n, err
}

// Write implements net.Conn.
func (l *Listener) Write(_ []byte) (int, error) {
	panic("unimplemented")
}

// LocalAddr implements net.Conn.
func (l *Listener) LocalAddr() net.Addr {
	panic("unimplemented")
}

// RemoteAddr implements net.Conn.
func (l *Listener) RemoteAddr() net.Addr {
	panic("unimplemented")
}

// SetDeadline implements net.Conn.
func (l *Listener) SetDeadline(_ time.Time) error {
	panic("unimplemented")
}

// SetReadDeadline implements net.Conn.
func (l *Listener) SetReadDeadline(t time.Time) error {
	return l.pc.SetReadDeadline(t)
}

// SetWriteDeadline implements net.Conn.
func (l *Listener) SetWriteDeadline(_ time.Time) error {
	panic("unimplemented")
}
