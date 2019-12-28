package rtsp

import (
	"net"
)

type Conn struct {
	c net.Conn
}

func NewConn(c net.Conn) *Conn {
	return &Conn{
		c: c,
	}
}

func (c *Conn) Close() error {
	return c.c.Close()
}

func (c *Conn) RemoteAddr() net.Addr {
	return c.c.RemoteAddr()
}

func (c *Conn) ReadRequest() (*Request, error) {
	return requestDecode(c.c)
}

func (c *Conn) WriteRequest(req *Request) error {
	return requestEncode(c.c, req)
}

func (c *Conn) ReadResponse() (*Response, error) {
	return responseDecode(c.c)
}

func (c *Conn) WriteResponse(res *Response) error {
	return responseEncode(c.c, res)
}
