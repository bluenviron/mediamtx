package websocket

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestClientConn(t *testing.T) {
	received := make(chan []byte, 1)

	s := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/mypath", r.URL.Path)
			require.Equal(t, "myvalue", r.Header.Get("X-Custom"))

			c, err := upgrader.Upgrade(w, r, nil)
			require.NoError(t, err)
			defer c.Close() //nolint:errcheck

			require.NoError(t, c.WriteMessage(websocket.BinaryMessage, []byte{1, 2, 3}))
			require.NoError(t, c.WriteMessage(websocket.TextMessage, []byte("text")))

			_, buf, err := c.ReadMessage()
			require.NoError(t, err)
			received <- buf
		}),
	}

	ln, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	go s.Serve(ln)
	defer s.Shutdown(context.Background())

	c, err := DialClientConn(context.Background(), "ws://"+ln.Addr().String()+"/mypath",
		nil, http.Header{"X-Custom": []string{"myvalue"}}, nil)
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	require.Equal(t, ln.Addr().String(), c.RemoteAddr().String())
	require.NotNil(t, c.LocalAddr())
	require.NoError(t, c.SetDeadline(time.Now().Add(5*time.Second)))
	require.NoError(t, c.SetReadDeadline(time.Now().Add(5*time.Second)))
	require.NoError(t, c.SetWriteDeadline(time.Now().Add(5*time.Second)))

	buf := make([]byte, 2)
	n, err := c.Read(buf)
	require.NoError(t, err)
	require.Equal(t, []byte{1, 2}, buf[:n])

	n, err = c.Read(buf)
	require.NoError(t, err)
	require.Equal(t, []byte{3}, buf[:n])

	_, err = c.Read(buf)
	require.EqualError(t, err, "unexpected message type 1")

	n, err = c.Write([]byte{4, 5})
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, []byte{4, 5}, <-received)
}
