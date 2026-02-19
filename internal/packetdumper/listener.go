package packetdumper

import "net"

var _ net.Listener = (*Listener)(nil)

// Listener is a wrapper around net.Listener that dumps packets to disk.
type Listener struct {
	Prefix   string
	Listener net.Listener
}

// Accept implements net.Listener.
func (l *Listener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	cd := &Conn{
		Prefix:     l.Prefix,
		Conn:       conn,
		ServerSide: true,
	}
	err = cd.Initialize()
	if err != nil {
		conn.Close() //nolint:errcheck
		return nil, err
	}

	return cd, nil
}

// Close implements net.Listener.
func (l *Listener) Close() error {
	return l.Listener.Close()
}

// Addr implements net.Listener.
func (l *Listener) Addr() net.Addr {
	return l.Listener.Addr()
}
