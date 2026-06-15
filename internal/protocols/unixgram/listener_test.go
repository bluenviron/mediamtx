package unixgram

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestListen(t *testing.T) {
	socket, err := os.CreateTemp(t.TempDir(), "mtx-unix-")
	require.NoError(t, err)
	socket.Close()

	l := &Listener{
		Path: socket.Name(),
	}
	err = l.Initialize()
	require.NoError(t, err)
	defer l.Close() //nolint:errcheck

	clientAddr, err := net.ResolveUnixAddr("unixgram", socket.Name())
	require.NoError(t, err)

	clientConn, err := net.DialUnix("unixgram", nil, clientAddr)
	require.NoError(t, err)
	defer clientConn.Close() //nolint:errcheck

	// Each datagram must be read back whole and in order: a datagram socket
	// preserves message boundaries.
	msgs := [][]byte{
		[]byte("first-datagram"),
		[]byte("2"),
		[]byte("the-third-and-longest-datagram"),
	}

	for _, m := range msgs {
		_, err = clientConn.Write(m)
		require.NoError(t, err)
	}

	buf := make([]byte, 1024)
	for _, m := range msgs {
		l.SetReadDeadline(time.Now().Add(2 * time.Second)) //nolint:errcheck
		n, err2 := l.Read(buf)
		require.NoError(t, err2)
		require.Equal(t, m, buf[:n])
	}
}
