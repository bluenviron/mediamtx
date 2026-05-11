package proxyprotocol

import (
	"net"

	proxyproto "github.com/pires/go-proxyproto"

	"github.com/bluenviron/mediamtx/internal/conf"
)

// WrapListener wraps a net.Listener with PROXY protocol support.
// Connections from trustedProxies have their PROXY header parsed if present.
// Connections from other IPs are passed through with zero overhead.
func WrapListener(ln net.Listener, trustedProxies conf.IPNetworks) net.Listener {
	return &proxyproto.Listener{
		Listener: ln,
		Policy: func(upstream net.Addr) (proxyproto.Policy, error) {
			tcpAddr, ok := upstream.(*net.TCPAddr)
			if ok && trustedProxies.Contains(tcpAddr.IP) {
				return proxyproto.USE, nil
			}
			return proxyproto.IGNORE, nil
		},
	}
}
