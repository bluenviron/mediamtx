package packetdumper

import "net"

var _ net.Listener = (*listener)(nil)

// listener is a wrapper around a net.Listener that dumps packets to disk.
type listener struct {
	Prefix   string
	Listener net.Listener
}

// Accept implements net.Listener.
func (l *listener) Accept() (net.Conn, error) {
	netConn, err := l.Listener.Accept()
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
func (l *listener) Close() error {
	return l.Listener.Close()
}

// Addr implements net.Listener.
func (l *listener) Addr() net.Addr {
	return l.Listener.Addr()
}
