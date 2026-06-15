// Package unix contains utilities to work with Unix sockets.
package unix

import (
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/readbuffer"
)

type packetConn interface {
	net.PacketConn
	SetReadBuffer(bytes int) error
	SyscallConn() (syscall.RawConn, error)
}

// Listener is a listener on a Unix socket.
//
// Datagram selects the socket type. RTP is packet-oriented and relies on
// message boundaries, so it uses a datagram socket ("unixgram"). MPEG-TS is a
// continuous byte stream, so it uses a stream socket ("unix").
type Listener struct {
	Path     string
	Datagram bool

	// datagram mode
	UDPReadBufferSize int
	ListenPacket      func(network string, address string) (net.PacketConn, error)

	// stream mode
	Listen func(network string, address string) (net.Listener, error)

	pc packetConn // datagram

	l        net.Listener // stream
	c        net.Conn
	mutex    sync.Mutex
	closed   bool
	deadline time.Time
}

// Initialize initializes the listener.
func (l *Listener) Initialize() error {
	if l.Path == "" {
		return fmt.Errorf("invalid unix path")
	}

	os.Remove(l.Path)

	if l.Datagram {
		if l.ListenPacket == nil {
			l.ListenPacket = net.ListenPacket
		}

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

	if l.Listen == nil {
		l.Listen = net.Listen
	}

	var err error
	l.l, err = l.Listen("unix", l.Path)
	return err
}

// Close closes the listener.
func (l *Listener) Close() error {
	if l.Datagram {
		err := l.pc.Close()
		// Datagram sockets are not unlinked automatically on close.
		os.Remove(l.Path) //nolint:errcheck
		return err
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.closed = true
	l.l.Close()
	if l.c != nil {
		l.c.Close()
	}
	return nil
}

func (l *Listener) acceptWithDeadline() (net.Conn, error) {
	done := make(chan struct{})
	defer func() { <-done }()

	terminate := make(chan struct{})
	defer close(terminate)

	go func() {
		defer close(done)
		select {
		case <-time.After(time.Until(l.deadline)):
			l.l.Close()
		case <-terminate:
			return
		}
	}()

	c, err := l.l.Accept()
	if err != nil {
		if time.Now().After(l.deadline) {
			return nil, fmt.Errorf("deadline exceeded")
		}
		return nil, err
	}
	return c, nil
}

func (l *Listener) setConn(c net.Conn) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.closed {
		return fmt.Errorf("closed")
	}

	l.c = c
	return nil
}

// Read implements net.Conn.
func (l *Listener) Read(p []byte) (int, error) {
	if l.Datagram {
		n, _, err := l.pc.ReadFrom(p)
		return n, err
	}

	if l.c == nil {
		c, err := l.acceptWithDeadline()
		if err != nil {
			return 0, err
		}

		err = l.setConn(c)
		if err != nil {
			return 0, err
		}
	}

	l.c.SetReadDeadline(l.deadline)
	return l.c.Read(p)
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
	if l.Datagram {
		return l.pc.SetReadDeadline(t)
	}
	l.deadline = t
	return nil
}

// SetWriteDeadline implements net.Conn.
func (l *Listener) SetWriteDeadline(_ time.Time) error {
	panic("unimplemented")
}
