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

func TestServerConn(t *testing.T) {
	pingReceived := make(chan struct{})
	pingInterval = 100 * time.Millisecond

	handler := func(w http.ResponseWriter, r *http.Request) {
		c, err := NewServerConn(w, r)
		require.NoError(t, err)
		defer c.Close()

		err = c.WriteJSON("testing")
		require.NoError(t, err)

		<-pingReceived
	}

	ln, err := net.Listen("tcp", "localhost:6344")
	require.NoError(t, err)
	defer ln.Close()

	s := &http.Server{Handler: http.HandlerFunc(handler)}
	go s.Serve(ln)
	defer s.Shutdown(context.Background())

	c, res, err := websocket.DefaultDialer.Dial("ws://localhost:6344/", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	defer c.Close()

	c.SetPingHandler(func(msg string) error {
		close(pingReceived)
		return nil
	})

	var msg string
	err = c.ReadJSON(&msg)
	require.NoError(t, err)
	require.Equal(t, "testing", msg)

	c.ReadMessage()

	<-pingReceived
}
