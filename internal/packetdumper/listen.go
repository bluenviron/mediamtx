package packetdumper

import (
	"net"
)

// Listen is a wrapper around net.Listen that dumps packets to disk.
type Listen struct {
	Prefix      string
	InnerListen func(network, address string) (net.Listener, error)
}

// Do mimics net.Listen.
func (l *Listen) Do(network, address string) (net.Listener, error) {
	listenFn := l.InnerListen
	if listenFn == nil {
		listenFn = net.Listen
	}

	netListener, err := listenFn(network, address)
	if err != nil {
		return nil, err
	}

	return &listener{
		Prefix:   l.Prefix,
		Listener: netListener,
	}, nil
}
