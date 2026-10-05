package udp_test

import (
	"fmt"
	"net"
	"strconv"
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

func TestListenMulticastAllInterfaces(t *testing.T) {
	probe, err := net.ListenPacket("udp4", "127.0.0.1:0")
	require.NoError(t, err)
	port := probe.LocalAddr().(*net.UDPAddr).Port
	probe.Close()

	address := net.JoinHostPort("238.0.0.1", strconv.Itoa(port))
	l := &Listener{
		Address:  address,
		IntfName: "all",
	}
	err = l.Initialize()
	require.NoError(t, err)
	defer l.Close() //nolint:errcheck
	require.Equal(t, "*multicast.multiConn", fmt.Sprintf("%T", l.pc))

	err = l.SetReadDeadline(time.Now().Add(2 * time.Second))
	require.NoError(t, err)

	clientConn, err := net.Dial("udp4", address)
	require.NoError(t, err)
	defer clientConn.Close() //nolint:errcheck

	_, err = clientConn.Write([]byte("testing"))
	require.NoError(t, err)

	buf := make([]byte, 1024)
	n, err := l.Read(buf)
	require.NoError(t, err)
	require.Equal(t, []byte("testing"), buf[:n])
}
