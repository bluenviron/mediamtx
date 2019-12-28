package rtsp

import (
	"net"
)

type Conn struct {
	net.Conn
}

func (c *Conn) ReadRequest() (*Request, error) {
	return requestDecode(c)
}

func (c *Conn) WriteRequest(req *Request) error {
	return requestEncode(c, req)
}

func (c *Conn) ReadResponse() (*Response, error) {
	return responseDecode(c)
}

func (c *Conn) WriteResponse(res *Response) error {
	return responseEncode(c, res)
}
