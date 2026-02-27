package packetdumper

import (
	"context"
	"net"
)

// DialContext is a wrapper around net.Dialer.DialContext that dumps packets to disk.
type DialContext struct {
	Prefix      string
	DialContext func(ctx context.Context, network, address string) (net.Conn, error)
}

// Do mimics net.Dialer.DialContext.
func (d *DialContext) Do(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := d.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}

	c := &Conn{
		Prefix: d.Prefix,
		Conn:   conn,
	}
	err = c.Initialize()
	if err != nil {
		conn.Close()
		return nil, err
	}

	return c, nil
}
