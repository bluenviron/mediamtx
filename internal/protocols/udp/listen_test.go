package udp

import (
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestListen(t *testing.T) {
	u, err := url.Parse("udp://127.0.0.1:0")
	require.NoError(t, err)

	conn, err := Listen(u, 4096)
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

	localAddr := conn.(*udpConn).pc.LocalAddr().(*net.UDPAddr)

	clientConn, err := net.DialUDP("udp", nil, localAddr)
	require.NoError(t, err)
	defer clientConn.Close() //nolint:errcheck

	_, err = clientConn.Write([]byte("testing"))
	require.NoError(t, err)

	<-done
}
