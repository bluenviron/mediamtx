package rtsp

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

type Conn struct {
	c        net.Conn
	writeBuf []byte
}

func NewConn(c net.Conn) *Conn {
	return &Conn{
		c:        c,
		writeBuf: make([]byte, 2048),
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

func (c *Conn) ReadInterleavedFrame(frame []byte) (int, int, error) {
	var header [4]byte
	_, err := io.ReadFull(c.c, header[:])
	if err != nil {
		return 0, 0, err
	}

	// connection terminated
	if header[0] == 0x54 {
		return 0, 0, io.EOF
	}

	if header[0] != 0x24 {
		return 0, 0, fmt.Errorf("wrong magic byte (0x%.2x)", header[0])
	}

	framelen := binary.BigEndian.Uint16(header[2:])
	if framelen > 2048 {
		return 0, 0, fmt.Errorf("frame length greater than 2048")
	}

	_, err = io.ReadFull(c.c, frame[:framelen])
	if err != nil {
		return 0, 0, err
	}

	return int(header[1]), int(framelen), nil
}

func (c *Conn) WriteInterleavedFrame(channel int, frame []byte) error {
	c.writeBuf[0] = 0x24
	c.writeBuf[1] = byte(channel)
	binary.BigEndian.PutUint16(c.writeBuf[2:], uint16(len(frame)))
	n := copy(c.writeBuf[4:], frame)

	_, err := c.c.Write(c.writeBuf[:4+n])
	if err != nil {
		return err
	}
	return nil
}

func (c *Conn) Read(buf []byte) (int, error) {
	return c.c.Read(buf)
}
