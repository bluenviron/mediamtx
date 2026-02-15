package unix

import (
	"net"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestListen(t *testing.T) {
	socket, err := os.CreateTemp(os.TempDir(), "mtx-unix-")
	require.NoError(t, err)
	socket.Close()
	defer os.Remove(socket.Name())

	u, err := url.Parse("unix://" + socket.Name())
	require.NoError(t, err)

	conn, err := Listen(u)
	require.NoError(t, err)
	defer conn.Close()

	done := make(chan struct{})

	go func() {
		defer close(done)

		buf := make([]byte, 1024)
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err2 := conn.Read(buf)
		require.NoError(t, err2)
		require.Equal(t, []byte("testing"), buf[:n])
	}()

	clientAddr, err := net.ResolveUnixAddr("unix", socket.Name())
	require.NoError(t, err)

	clientConn, err := net.DialUnix("unix", nil, clientAddr)
	require.NoError(t, err)
	defer clientConn.Close() //nolint:errcheck

	_, err = clientConn.Write([]byte("testing"))
	require.NoError(t, err)

	<-done
}
