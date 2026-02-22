package packetdumper

import (
	"net"
)

// ListenPacket is a wrapper around net.ListenPacket that dumps packets to disk.
type ListenPacket struct {
	Prefix       string
	ListenPacket func(network, address string) (net.PacketConn, error)
}

// Do mimics net.ListenPacket
func (l *ListenPacket) Do(network, address string) (net.PacketConn, error) {
	pc, err := l.ListenPacket(network, address)
	if err != nil {
		return nil, err
	}

	d := &PacketConn{
		Prefix:     l.Prefix,
		PacketConn: pc,
	}
	err = d.Initialize()
	if err != nil {
		return nil, err
	}

	return d, nil
}
