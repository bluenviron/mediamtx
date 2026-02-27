package unix

import (
	"net"
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

	l := &Listener{
		Path: socket.Name(),
	}
	err = l.Initialize()
	require.NoError(t, err)
	defer l.Close() //nolint:errcheck

	done := make(chan struct{})

	go func() {
		defer close(done)

		buf := make([]byte, 1024)
		l.SetReadDeadline(time.Now().Add(2 * time.Second)) //nolint:errcheck
		n, err2 := l.Read(buf)
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
