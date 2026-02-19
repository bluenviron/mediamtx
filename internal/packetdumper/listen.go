package packetdumper

import (
	"net"
)

// Listen is a wrapper around net.Listen that dumps packets to disk.
type Listen struct {
	Prefix string
	Listen func(network, address string) (net.Listener, error)
}

// Do mimics net.Listen.
func (l *Listen) Do(network, address string) (net.Listener, error) {
	ln, err := l.Listen(network, address)
	if err != nil {
		return nil, err
	}

	return &Listener{
		Prefix:   l.Prefix,
		Listener: ln,
	}, nil
}
