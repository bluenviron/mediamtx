package packetdumper

import (
	"net"
)

// ListenPacket is a wrapper around net.ListenPacket that dumps packets to disk.
type ListenPacket struct {
	Prefix string
}

// Do mimics net.ListenPacket.
func (l *ListenPacket) Do(network, address string) (net.PacketConn, error) {
	netPacketConn, err := net.ListenPacket(network, address)
	if err != nil {
		return nil, err
	}

	pdPacketConn := &packetConn{
		Prefix:     l.Prefix,
		PacketConn: netPacketConn,
	}
	err = pdPacketConn.Initialize()
	if err != nil {
		netPacketConn.Close()
		return nil, err
	}

	return pdPacketConn, nil
}
