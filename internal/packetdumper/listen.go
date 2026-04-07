package packetdumper

import (
	"net"
)

// Listen is a wrapper around net.Listen that dumps packets to disk.
type Listen struct {
	Prefix string
}

// Do mimics net.Listen.
func (l *Listen) Do(network, address string) (net.Listener, error) {
	netListener, err := net.Listen(network, address)
	if err != nil {
		return nil, err
	}

	return &listener{
		Prefix:   l.Prefix,
		Listener: netListener,
	}, nil
}
