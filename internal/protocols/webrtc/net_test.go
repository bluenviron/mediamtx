package webrtc

import (
	"net"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNetListenUDPWriteBufferSize(t *testing.T) {
	n := &Net{
		UDPWriteBufferSize: 65536,
	}

	conn, err := n.ListenUDP("udp4", &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 0,
	})
	require.NoError(t, err)
	defer conn.Close()

	udpConn := conn.(*net.UDPConn)
	rawConn, err := udpConn.SyscallConn()
	require.NoError(t, err)

	var (
		writeBufferSize int
		getsockoptErr   error
	)
	err = rawConn.Control(func(fd uintptr) {
		writeBufferSize, getsockoptErr = syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF)
	})
	require.NoError(t, err)
	require.NoError(t, getsockoptErr)
	require.GreaterOrEqual(t, writeBufferSize, n.UDPWriteBufferSize)
}
