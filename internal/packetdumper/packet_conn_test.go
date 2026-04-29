package packetdumper

import (
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// startUDPPair creates a pair of UDP connections and returns both ends.
func startUDPPair(t *testing.T) (client, server *net.UDPConn) {
	t.Helper()

	serverAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)

	server, err = net.ListenUDP("udp", serverAddr)
	require.NoError(t, err)

	clientAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)

	client, err = net.ListenUDP("udp", clientAddr)
	require.NoError(t, err)

	t.Cleanup(func() {
		server.Close() //nolint:errcheck
		client.Close() //nolint:errcheck
	})

	return client, server
}

func TestPacketConnInitialize_CreatesFile(t *testing.T) {
	client, server := startUDPPair(t)

	prefix := filepath.Join(t.TempDir(), "capture")
	c := &packetConn{Prefix: prefix, PacketConn: client}
	require.NoError(t, c.Initialize())

	c.Close()      //nolint:errcheck
	server.Close() //nolint:errcheck

	checkPcapngPresence(t, prefix)
}

func TestPacketConnWriteTo(t *testing.T) {
	client, server := startUDPPair(t)

	prefix := filepath.Join(t.TempDir(), "capture")
	c := &packetConn{Prefix: prefix, PacketConn: client}
	require.NoError(t, c.Initialize())

	n, err := c.WriteTo([]byte("hello world"), server.LocalAddr())
	require.NoError(t, err)
	require.Equal(t, 11, n)

	buf := make([]byte, 32)
	server.SetReadDeadline(time.Now().Add(2 * time.Second)) //nolint:errcheck
	rn, _, err := server.ReadFromUDP(buf)
	require.NoError(t, err)
	require.Equal(t, []byte("hello world"), buf[:rn])

	c.Close()      //nolint:errcheck
	server.Close() //nolint:errcheck

	checkPcapngPresence(t, prefix)
}

func TestPacketConnReadFrom(t *testing.T) {
	client, server := startUDPPair(t)

	prefix := filepath.Join(t.TempDir(), "capture")
	c := &packetConn{Prefix: prefix, PacketConn: client}
	require.NoError(t, c.Initialize())

	_, err := server.WriteTo([]byte("incoming data"), client.LocalAddr())
	require.NoError(t, err)

	buf := make([]byte, 32)
	c.SetReadDeadline(time.Now().Add(2 * time.Second)) //nolint:errcheck
	n, addr, err := c.ReadFrom(buf)
	require.NoError(t, err)
	require.Equal(t, []byte("incoming data"), buf[:n])
	require.NotNil(t, addr)

	c.Close()      //nolint:errcheck
	server.Close() //nolint:errcheck

	checkPcapngPresence(t, prefix)
}

func TestPacketConnMultipleWriteRead(t *testing.T) {
	client, server := startUDPPair(t)

	prefix := filepath.Join(t.TempDir(), "capture")
	c := &packetConn{Prefix: prefix, PacketConn: client}
	require.NoError(t, c.Initialize())

	serverAddr := server.LocalAddr()
	for _, msg := range []string{"foo", "bar", "baz"} {
		n, err := c.WriteTo([]byte(msg), serverAddr)
		require.NoError(t, err)
		require.Equal(t, len(msg), n)
	}

	server.SetReadDeadline(time.Now().Add(2 * time.Second)) //nolint:errcheck
	buf := make([]byte, 32)
	received := make([]byte, 0, 9)
	for range 3 {
		n, _, err := server.ReadFromUDP(buf)
		require.NoError(t, err)
		received = append(received, buf[:n]...)
	}
	require.Equal(t, []byte("foobarbaz"), received)

	for _, msg := range []string{"abcde", "fghij"} {
		_, err := server.WriteTo([]byte(msg), client.LocalAddr())
		require.NoError(t, err)
	}

	c.SetReadDeadline(time.Now().Add(2 * time.Second)) //nolint:errcheck
	readReceived := make([]byte, 0, 10)
	for range 2 {
		n, _, err := c.ReadFrom(buf)
		require.NoError(t, err)
		readReceived = append(readReceived, buf[:n]...)
	}
	require.Equal(t, []byte("abcdefghij"), readReceived)

	c.Close()      //nolint:errcheck
	server.Close() //nolint:errcheck

	checkPcapngPresence(t, prefix)
}

func TestPacketConnCloseIdempotent(t *testing.T) {
	client, server := startUDPPair(t)

	prefix := filepath.Join(t.TempDir(), "capture")
	c := &packetConn{Prefix: prefix, PacketConn: client}
	require.NoError(t, c.Initialize())

	c.Close()      //nolint:errcheck
	c.Close()      //nolint:errcheck
	server.Close() //nolint:errcheck

	checkPcapngPresence(t, prefix)
}

func TestPacketConnDelegatesAddrMethods(t *testing.T) {
	client, server := startUDPPair(t)

	prefix := filepath.Join(t.TempDir(), "capture")
	c := &packetConn{Prefix: prefix, PacketConn: client}
	require.NoError(t, c.Initialize())

	require.Equal(t, client.LocalAddr(), c.LocalAddr())

	require.NoError(t, c.SetDeadline(time.Now().Add(time.Second)))
	require.NoError(t, c.SetReadDeadline(time.Now().Add(time.Second)))
	require.NoError(t, c.SetWriteDeadline(time.Now().Add(time.Second)))

	c.Close()      //nolint:errcheck
	server.Close() //nolint:errcheck

	checkPcapngPresence(t, prefix)
}

func TestPacketConnReadFromRecordsSource(t *testing.T) {
	client, server := startUDPPair(t)

	prefix := filepath.Join(t.TempDir(), "capture")
	c := &packetConn{Prefix: prefix, PacketConn: client}
	require.NoError(t, c.Initialize())

	_, err := server.WriteTo([]byte("ping"), client.LocalAddr())
	require.NoError(t, err)

	buf := make([]byte, 32)
	c.SetReadDeadline(time.Now().Add(2 * time.Second)) //nolint:errcheck
	n, addr, err := c.ReadFrom(buf)
	require.NoError(t, err)
	require.Equal(t, []byte("ping"), buf[:n])

	// The reported source address should match the server's address.
	require.Equal(t, server.LocalAddr().String(), addr.String())

	c.Close()      //nolint:errcheck
	server.Close() //nolint:errcheck

	checkPcapngPresence(t, prefix)
}
