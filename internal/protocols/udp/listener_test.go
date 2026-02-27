package udp

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestListen(t *testing.T) {
	l := &Listener{
		Address:           "127.0.0.1:0",
		UDPReadBufferSize: 4096,
	}
	err := l.Initialize()
	require.NoError(t, err)
	defer l.Close() //nolint:errcheck

	done := make(chan struct{})

	go func() {
		defer close(done)

		err2 := l.SetReadDeadline(time.Now().Add(2 * time.Second))
		require.NoError(t, err2)

		buf := make([]byte, 1024)
		n, err2 := l.Read(buf)
		require.NoError(t, err2)

		require.Equal(t, []byte("testing"), buf[:n])
	}()

	localAddr := l.pc.LocalAddr().(*net.UDPAddr)

	clientConn, err := net.DialUDP("udp", nil, localAddr)
	require.NoError(t, err)
	defer clientConn.Close() //nolint:errcheck

	_, err = clientConn.Write([]byte("testing"))
	require.NoError(t, err)

	<-done
}
