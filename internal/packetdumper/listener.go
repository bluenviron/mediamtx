package packetdumper

import "net"

var _ net.Listener = (*Listener)(nil)

// Listener is a wrapper around a net.Listener that dumps packets to disk.
type Listener struct {
	Wrapped net.Listener
	Prefix  string
}

// Accept implements net.Listener.
func (l *Listener) Accept() (net.Conn, error) {
	netConn, err := l.Wrapped.Accept()
	if err != nil {
		return nil, err
	}

	pdConn := &conn{
		Prefix:     l.Prefix,
		Conn:       netConn,
		ServerSide: true,
	}
	err = pdConn.Initialize()
	if err != nil {
		netConn.Close() //nolint:errcheck
		return nil, err
	}

	return pdConn, nil
}

// Close implements net.Listener.
func (l *Listener) Close() error {
	return l.Wrapped.Close()
}

// Addr implements net.Listener.
func (l *Listener) Addr() net.Addr {
	return l.Wrapped.Addr()
}
