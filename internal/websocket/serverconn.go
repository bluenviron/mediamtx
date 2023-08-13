// Package websocket provides WebSocket connectivity.
package websocket

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var (
	pingInterval = 30 * time.Second
	pingTimeout  = 5 * time.Second
	writeTimeout = 2 * time.Second
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// ServerConn is a server-side WebSocket connection with
// automatic, periodic ping-pong
type ServerConn struct {
	wc *websocket.Conn

	// in
	terminate chan struct{}
	write     chan []byte

	// out
	writeErr chan error
}

// NewServerConn allocates a ServerConn.
func NewServerConn(w http.ResponseWriter, req *http.Request) (*ServerConn, error) {
	wc, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return nil, err
	}

	c := &ServerConn{
		wc:        wc,
		terminate: make(chan struct{}),
		write:     make(chan []byte),
		writeErr:  make(chan error),
	}

	go c.run()

	return c, nil
}

// Close closes a ServerConn.
func (c *ServerConn) Close() {
	c.wc.Close() //nolint:errcheck
	close(c.terminate)
}

// RemoteAddr returns the remote address.
func (c *ServerConn) RemoteAddr() net.Addr {
	return c.wc.RemoteAddr()
}

func (c *ServerConn) run() {
	c.wc.SetReadDeadline(time.Now().Add(pingInterval + pingTimeout)) //nolint:errcheck

	c.wc.SetPongHandler(func(string) error {
		c.wc.SetReadDeadline(time.Now().Add(pingInterval + pingTimeout)) //nolint:errcheck
		return nil
	})

	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()

	for {
		select {
		case byts := <-c.write:
			c.wc.SetWriteDeadline(time.Now().Add(writeTimeout)) //nolint:errcheck
			err := c.wc.WriteMessage(websocket.TextMessage, byts)
			c.writeErr <- err

		case <-pingTicker.C:
			c.wc.SetWriteDeadline(time.Now().Add(writeTimeout)) //nolint:errcheck
			c.wc.WriteMessage(websocket.PingMessage, nil)       //nolint:errcheck

		case <-c.terminate:
			return
		}
	}
}

// ReadJSON reads a JSON object.
func (c *ServerConn) ReadJSON(in interface{}) error {
	return c.wc.ReadJSON(in)
}

// WriteJSON writes a JSON object.
func (c *ServerConn) WriteJSON(in interface{}) error {
	byts, err := json.Marshal(in)
	if err != nil {
		return err
	}

	select {
	case c.write <- byts:
		return <-c.writeErr
	case <-c.terminate:
		return fmt.Errorf("terminated")
	}
}
