package websocket

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ClientConn is a client-side WebSocket connection that exposes
// binary messages as a byte stream.
type ClientConn struct {
	wc *websocket.Conn

	readBuf    []byte
	writeMutex sync.Mutex
}

// DialClientConn opens a WebSocket connection.
func DialClientConn(
	ctx context.Context,
	url string,
	subprotocols []string,
	header http.Header,
	tlsConfig *tls.Config,
) (*ClientConn, error) {
	dialer := &websocket.Dialer{
		Subprotocols:    subprotocols,
		TLSClientConfig: tlsConfig,
	}

	wc, _, err := dialer.DialContext(ctx, url, header) //nolint:bodyclose
	if err != nil {
		return nil, err
	}

	return &ClientConn{wc: wc}, nil
}

// Read implements net.Conn.
func (c *ClientConn) Read(p []byte) (int, error) {
	if len(c.readBuf) == 0 {
		msgType, buf, err := c.wc.ReadMessage()
		if err != nil {
			return 0, err
		}

		if msgType != websocket.BinaryMessage {
			return 0, fmt.Errorf("unexpected message type %v", msgType)
		}

		c.readBuf = buf
	}

	n := copy(p, c.readBuf)
	c.readBuf = c.readBuf[n:]

	return n, nil
}

// Write implements net.Conn.
func (c *ClientConn) Write(p []byte) (int, error) {
	c.writeMutex.Lock()
	defer c.writeMutex.Unlock()

	err := c.wc.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

// Close implements net.Conn.
func (c *ClientConn) Close() error {
	return c.wc.Close()
}

// LocalAddr implements net.Conn.
func (c *ClientConn) LocalAddr() net.Addr {
	return c.wc.LocalAddr()
}

// RemoteAddr implements net.Conn.
func (c *ClientConn) RemoteAddr() net.Addr {
	return c.wc.RemoteAddr()
}

// SetDeadline implements net.Conn.
func (c *ClientConn) SetDeadline(t time.Time) error {
	return c.wc.NetConn().SetDeadline(t)
}

// SetReadDeadline implements net.Conn.
func (c *ClientConn) SetReadDeadline(t time.Time) error {
	return c.wc.SetReadDeadline(t)
}

// SetWriteDeadline implements net.Conn.
func (c *ClientConn) SetWriteDeadline(t time.Time) error {
	return c.wc.SetWriteDeadline(t)
}
