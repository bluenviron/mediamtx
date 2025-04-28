// Package restrictnetwork contains Restrict().
package restrictnetwork

import (
	"net"
)

// Restrict prevents listening on IPv6 when address is 0.0.0.0.
func Restrict(network string, address string) (string, string) {
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		if host == "0.0.0.0" {
			return network + "4", address
		}
	}

	return network, address
}
