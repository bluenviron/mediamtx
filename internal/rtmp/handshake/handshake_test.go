package handshake

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandshake(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:9122")
	require.NoError(t, err)
	defer ln.Close()

	done := make(chan struct{})

	go func() {
		conn, err := ln.Accept()
		require.NoError(t, err)
		defer conn.Close()

		err = DoServer(conn, true)
		require.NoError(t, err)

		close(done)
	}()

	conn, err := net.Dial("tcp", "127.0.0.1:9122")
	require.NoError(t, err)
	defer conn.Close()

	err = DoClient(conn, true)
	require.NoError(t, err)

	<-done
}
