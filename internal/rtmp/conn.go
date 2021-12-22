package rtmp

import (
	"net"
	"net/url"
	"time"

	"github.com/notedit/rtmp/av"
	"github.com/notedit/rtmp/format/rtmp"
)

// Conn is a RTMP connection.
type Conn struct {
	rconn *rtmp.Conn
	nconn net.Conn
}

// Close closes the connection.
func (c *Conn) Close() error {
	return c.nconn.Close()
}

// SetReadDeadline sets the read deadline.
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.nconn.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.nconn.SetWriteDeadline(t)
}

// RemoteAddr returns the remote network address.
func (c *Conn) RemoteAddr() net.Addr {
	return c.nconn.RemoteAddr()
}

// IsPublishing returns whether the connection is publishing.
func (c *Conn) IsPublishing() bool {
	return c.rconn.Publishing
}

// URL returns the URL requested by the connection.
func (c *Conn) URL() *url.URL {
	return c.rconn.URL
}

// ReadPacket reads a packet.
func (c *Conn) ReadPacket() (av.Packet, error) {
	return c.rconn.ReadPacket()
}

// WritePacket writes a packet.
func (c *Conn) WritePacket(pkt av.Packet) error {
	err := c.rconn.WritePacket(pkt)
	if err != nil {
		return err
	}
	return c.rconn.FlushWrite()
}
