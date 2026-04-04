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

type tlsListener struct {
	Listener  net.Listener
	TLSConfig *tls.Config
}

func (l *tlsListener) Close() error {
	return l.Listener.Close()
}

func (l *tlsListener) Addr() net.Addr {
	return l.Listener.Addr()
}

func (l *tlsListener) Accept() (net.Conn, error) {
	netConn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	tlsConfig := l.TLSConfig.Clone()
	pdConn := netConn.(*conn)
	pdConn.expectingSecrets = 4
	tlsConfig.KeyLogWriter = &connKeyLogWriter{c: pdConn}

	return tls.Server(netConn, tlsConfig), nil
}
