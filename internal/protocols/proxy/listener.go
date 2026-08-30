// Package proxy provides PROXY protocol support for net.Listener.
package proxy

import (
	"net"

	"github.com/pires/go-proxyproto"

	"github.com/bluenviron/mediamtx/internal/conf"
)

var _ net.Listener = (*Listener)(nil)

// Listener is a net.Listener that supports PROXY protocol.
type Listener struct {
	Wrapped        net.Listener
	TrustedProxies conf.IPNetworks

	inner *proxyproto.Listener
}

// Initialize initializes the listener.
func (l *Listener) Initialize() {
	l.inner = &proxyproto.Listener{
		Listener: l.Wrapped,
		ConnPolicy: func(connPolicyOptions proxyproto.ConnPolicyOptions) (proxyproto.Policy, error) {
			tcpAddr, ok := connPolicyOptions.Upstream.(*net.TCPAddr)
			if ok && l.TrustedProxies.Contains(tcpAddr.IP) {
				return proxyproto.USE, nil
			}
			return proxyproto.IGNORE, nil
		},
	}
}

// Close implements net.Listener.
func (l *Listener) Close() error {
	return l.Wrapped.Close()
}

// Accept implements net.Listener.
func (l *Listener) Accept() (net.Conn, error) {
	return l.inner.Accept()
}

// Addr implements net.Listener.
func (l *Listener) Addr() net.Addr {
	return l.Wrapped.Addr()
}
