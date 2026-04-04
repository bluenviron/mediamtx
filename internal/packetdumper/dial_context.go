package packetdumper

import (
	"context"
	"net"
)

// DialContext is a wrapper around net.Dialer.DialContext that dumps packets to disk.
type DialContext struct {
	Prefix string
}

// Do mimics net.Dialer.DialContext.
func (d *DialContext) Do(ctx context.Context, network, address string) (net.Conn, error) {
	netConn, err := (&net.Dialer{}).DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}

	pdConn := &conn{
		Prefix: d.Prefix,
		Conn:   netConn,
	}
	err = pdConn.Initialize()
	if err != nil {
		netConn.Close()
		return nil, err
	}

	return pdConn, nil
}
