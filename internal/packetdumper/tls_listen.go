package packetdumper

import (
	"crypto/tls"
	"net"
)

// TLSListen provides a tls.Listen that also dumps TLS master secrets to disk.
type TLSListen struct {
	Listen func(network string, address string) (net.Listener, error)
}

// Do mimics tls.Listen.
func (l *TLSListen) Do(network string, laddr string, tlsConfig *tls.Config) (net.Listener, error) {
	netListener, err := l.Listen(network, laddr)
	if err != nil {
		return nil, err
	}

	return &tlsListener{
		Listener:  netListener,
		TLSConfig: tlsConfig,
	}, nil
}
