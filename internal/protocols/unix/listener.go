// Package unix contains utilities to work with Unix sockets.
package unix

import (
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

// Listener is a listener on a Unix socket.
type Listener struct {
	Path string

	l        net.Listener
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

	var err error
	l.l, err = net.Listen("unix", l.Path)
	if err != nil {
		return err
	}

	return nil
}

// Close closes the listener.
func (l *Listener) Close() error {
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

func (l *Listener) Read(p []byte) (int, error) {
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
	l.deadline = t
	return nil
}

// SetWriteDeadline implements net.Conn.
func (l *Listener) SetWriteDeadline(_ time.Time) error {
	panic("unimplemented")
}
