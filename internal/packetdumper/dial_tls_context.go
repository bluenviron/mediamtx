package packetdumper

import (
	"context"
	"crypto/tls"
	"net"
)

// DialTLSContext provides the DialTLSContext function.
type DialTLSContext struct {
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)
	TLSConfig   *tls.Config
}

// Do provides DialTLSContext.
func (t *DialTLSContext) Do(ctx context.Context, network, addr string) (net.Conn, error) {
	netConn, err := t.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	// clone TLS config and fill ServerName if empty.
	// this is the same behavior of http.Client.
	// https://cs.opensource.google/go/go/+/master:src/net/http/transport.go;l=1754;drc=a4b534f5e42fe58d58c0ff0562d76680cedb0466
	tlsConfig := t.TLSConfig

	if tlsConfig == nil {
		tlsConfig = &tls.Config{}
	} else {
		tlsConfig = tlsConfig.Clone()
	}

	if tlsConfig.ServerName == "" {
		host, _, _ := net.SplitHostPort(addr)
		tlsConfig.ServerName = host
	}

	pdConn := netConn.(*conn)
	pdConn.expectingSecrets = 4
	tlsConfig.KeyLogWriter = &connKeyLogWriter{c: pdConn}

	return tls.Client(netConn, tlsConfig), nil
}
