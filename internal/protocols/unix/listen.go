// Package unix contains utilities to work with Unix sockets.
package unix

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"sync"
	"time"
)

type unixConn struct {
	l        net.Listener
	c        net.Conn
	mutex    sync.Mutex
	closed   bool
	deadline time.Time
}

func (r *unixConn) Close() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.closed = true

	r.l.Close()

	if r.c != nil {
		r.c.Close()
	}

	return nil
}

func (r *unixConn) acceptWithDeadline() (net.Conn, error) {
	done := make(chan struct{})
	defer func() { <-done }()

	terminate := make(chan struct{})
	defer close(terminate)

	go func() {
		defer close(done)
		select {
		case <-time.After(time.Until(r.deadline)):
			r.l.Close()
		case <-terminate:
			return
		}
	}()

	c, err := r.l.Accept()
	if err != nil {
		if time.Now().After(r.deadline) {
			return nil, fmt.Errorf("deadline exceeded")
		}
		return nil, err
	}
	return c, nil
}

func (r *unixConn) setConn(c net.Conn) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.closed {
		return fmt.Errorf("closed")
	}

	r.c = c
	return nil
}

func (r *unixConn) Read(p []byte) (int, error) {
	if r.c == nil {
		c, err := r.acceptWithDeadline()
		if err != nil {
			return 0, err
		}

		err = r.setConn(c)
		if err != nil {
			return 0, err
		}
	}

	r.c.SetReadDeadline(r.deadline)
	return r.c.Read(p)
}

func (r *unixConn) Write(_ []byte) (int, error) {
	panic("unimplemented")
}

func (r *unixConn) LocalAddr() net.Addr {
	panic("unimplemented")
}

func (r *unixConn) RemoteAddr() net.Addr {
	panic("unimplemented")
}

func (r *unixConn) SetDeadline(_ time.Time) error {
	panic("unimplemented")
}

func (r *unixConn) SetReadDeadline(t time.Time) error {
	r.deadline = t
	return nil
}

func (r *unixConn) SetWriteDeadline(_ time.Time) error {
	panic("unimplemented")
}

// Listen creates a Unix listener on the given URL.
func Listen(u *url.URL) (net.Conn, error) {
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

	socket, err := net.Listen("unix", pa)
	if err != nil {
		return nil, err
	}

	return &unixConn{l: socket}, nil
}
