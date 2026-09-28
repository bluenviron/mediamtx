package udp_test

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/protocols/udp"
)

func newListener(t *testing.T) (*udp.Listener, *net.UDPAddr) {
	t.Helper()

	var localAddr *net.UDPAddr
	l := &udp.Listener{
		Address:           "127.0.0.1:0",
		UDPReadBufferSize: 4096,
		ListenPacket: func(network, address string) (net.PacketConn, error) {
			pc, err := net.ListenPacket(network, address)
			if err == nil {
				localAddr = pc.LocalAddr().(*net.UDPAddr)
			}
			return pc, err
		},
	}
	err := l.Initialize()
	require.NoError(t, err)
	t.Cleanup(func() { l.Close() }) //nolint:errcheck

	return l, localAddr
}

func TestListen(t *testing.T) {
	l, localAddr := newListener(t)
	err := l.SetReadDeadline(time.Now().Add(2 * time.Second))
	require.NoError(t, err)

	clientConn, err := net.DialUDP("udp", nil, localAddr)
	require.NoError(t, err)
	defer clientConn.Close() //nolint:errcheck

	_, err = clientConn.Write([]byte("testing"))
	require.NoError(t, err)

	buf := make([]byte, 1024)
	n, err := l.Read(buf)
	require.NoError(t, err)
	require.Equal(t, []byte("testing"), buf[:n])
}

func TestListenEmptyDatagram(t *testing.T) {
	l, localAddr := newListener(t)
	err := l.SetReadDeadline(time.Now().Add(2 * time.Second))
	require.NoError(t, err)

	clientConn, err := net.DialUDP("udp", nil, localAddr)
	require.NoError(t, err)
	defer clientConn.Close() //nolint:errcheck

	_, err = clientConn.Write(nil)
	require.NoError(t, err)
	_, err = clientConn.Write([]byte("testing"))
	require.NoError(t, err)

	buf := make([]byte, 1024)
	n, err := l.Read(buf)
	require.NoError(t, err)
	require.Equal(t, []byte("testing"), buf[:n])
}

func TestListenEmptyDatagramDeadline(t *testing.T) {
	l, localAddr := newListener(t)
	err := l.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	require.NoError(t, err)

	clientConn, err := net.DialUDP("udp", nil, localAddr)
	require.NoError(t, err)
	defer clientConn.Close() //nolint:errcheck

	_, err = clientConn.Write(nil)
	require.NoError(t, err)

	buf := make([]byte, 1024)
	n, err := l.Read(buf)
	require.Zero(t, n)
	var netErr net.Error
	require.ErrorAs(t, err, &netErr)
	require.True(t, netErr.Timeout())
}
