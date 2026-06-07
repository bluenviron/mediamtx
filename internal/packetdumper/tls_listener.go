package packetdumper

import (
	"crypto/tls"
	"net"
	"time"
)

type connKeyLogWriter struct {
	c *conn
}

func (w *connKeyLogWriter) Write(p []byte) (int, error) {
	w.c.enqueue(dumpEntry{
		ntp:       time.Now(),
		data:      append([]byte(nil), p...),
		direction: dirSecret,
	})

	return len(p), nil
}

var _ net.Listener = (*TLSListener)(nil)

// TLSListener is a wrapper around a net.Listener that dumps TLS master secrets to disk.
type TLSListener struct {
	Wrapped   net.Listener
	TLSConfig *tls.Config
}

// Close implements net.Listener.
func (l *TLSListener) Close() error {
	return l.Wrapped.Close()
}

// Addr implements net.Listener.
func (l *TLSListener) Addr() net.Addr {
	return l.Wrapped.Addr()
}

// Accept implements net.Listener.
func (l *TLSListener) Accept() (net.Conn, error) {
	netConn, err := l.Wrapped.Accept()
	if err != nil {
		return nil, err
	}

	tlsConfig := l.TLSConfig.Clone()
	pdConn := netConn.(*conn)
	pdConn.expectingSecrets.Store(4)
	tlsConfig.KeyLogWriter = &connKeyLogWriter{c: pdConn}

	return tls.Server(netConn, tlsConfig), nil
}
