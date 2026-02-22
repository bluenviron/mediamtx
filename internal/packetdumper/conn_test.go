package packetdumper

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// startTCPPair dials a local TCP listener and returns both ends of the connection.
func startTCPPair(t *testing.T) (client, server net.Conn) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	serverCh := make(chan net.Conn, 1)
	go func() {
		conn, err2 := ln.Accept()
		if err2 == nil {
			serverCh <- conn
		}
	}()

	client, err = net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)

	t.Cleanup(func() { ln.Close() })

	select {
	case server = <-serverCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server connection")
	}

	return client, server
}

func cleanupPcapng(t *testing.T, prefix string) {
	t.Helper()

	matches, err := filepath.Glob(prefix + "_*.pcapng")
	require.NoError(t, err, "glob for pcapng files")
	require.NotEmpty(t, matches, "expected at least one pcapng file to have been created")

	for _, f := range matches {
		require.NoError(t, os.Remove(f), "removing pcapng file %s", f)
	}
}

func TestConnInitialize_CreatesFile(t *testing.T) {
	client, server := startTCPPair(t)
	defer server.Close()

	prefix := filepath.Join(t.TempDir(), "capture")
	c := &Conn{Prefix: prefix, Conn: client}
	require.NoError(t, c.Initialize())

	defer cleanupPcapng(t, prefix)
	defer c.Close() //nolint:errcheck
}

func TestConnWrite(t *testing.T) {
	client, server := startTCPPair(t)
	defer server.Close()

	prefix := filepath.Join(t.TempDir(), "capture")
	c := &Conn{Prefix: prefix, Conn: client}
	require.NoError(t, c.Initialize())

	defer cleanupPcapng(t, prefix)
	defer c.Close() //nolint:errcheck

	n, err := c.Write([]byte("hello world"))
	require.NoError(t, err)
	require.Equal(t, 11, n)

	buf := make([]byte, 11)
	_, err = io.ReadFull(server, buf)
	require.NoError(t, err)
	require.Equal(t, []byte("hello world"), buf)
}

func TestConnRead(t *testing.T) {
	client, server := startTCPPair(t)
	defer server.Close()

	prefix := filepath.Join(t.TempDir(), "capture")
	c := &Conn{Prefix: prefix, Conn: client}
	require.NoError(t, c.Initialize())

	defer cleanupPcapng(t, prefix)
	defer c.Close() //nolint:errcheck

	_, err := server.Write([]byte("incoming data"))
	require.NoError(t, err)

	buf := make([]byte, 32)
	n, err := c.Read(buf)
	require.NoError(t, err)
	require.Equal(t, []byte("incoming data"), buf[:n])
}

func TestConnServerSide(t *testing.T) {
	client, server := startTCPPair(t)
	defer client.Close()

	prefix := filepath.Join(t.TempDir(), "capture")
	c := &Conn{Prefix: prefix, Conn: server, ServerSide: true}
	require.NoError(t, c.Initialize())

	defer cleanupPcapng(t, prefix)
	defer c.Close() //nolint:errcheck

	n, err := c.Write([]byte("server response"))
	require.NoError(t, err)
	require.Equal(t, 15, n)

	buf := make([]byte, 15)
	_, err = io.ReadFull(client, buf)
	require.NoError(t, err)
	require.Equal(t, []byte("server response"), buf)
}

func TestConnMultipleWriteRead(t *testing.T) {
	client, server := startTCPPair(t)
	defer server.Close()

	prefix := filepath.Join(t.TempDir(), "capture")
	c := &Conn{Prefix: prefix, Conn: client}
	require.NoError(t, c.Initialize())

	defer cleanupPcapng(t, prefix)
	defer c.Close() //nolint:errcheck

	for _, msg := range []string{"foo", "bar", "baz"} {
		n, err := c.Write([]byte(msg))
		require.NoError(t, err)
		require.Equal(t, len(msg), n)
	}

	buf := make([]byte, len("foobarbaz"))
	_, err := io.ReadFull(server, buf)
	require.NoError(t, err)
	require.Equal(t, []byte("foobarbaz"), buf)

	_, err = server.Write([]byte("abcde"))
	require.NoError(t, err)
	_, err = server.Write([]byte("fghij"))
	require.NoError(t, err)

	readBuf := make([]byte, 10)
	_, err = io.ReadFull(c, readBuf)
	require.NoError(t, err)
	require.Equal(t, []byte("abcdefghij"), readBuf)
}

func TestConnCloseIdempotent(t *testing.T) {
	client, server := startTCPPair(t)
	defer server.Close()

	prefix := filepath.Join(t.TempDir(), "capture")
	c := &Conn{Prefix: prefix, Conn: client}
	require.NoError(t, c.Initialize())

	defer cleanupPcapng(t, prefix)

	defer c.Close() //nolint:errcheck
	defer c.Close() //nolint:errcheck
}

func TestConnDelegatesAddrMethods(t *testing.T) {
	client, server := startTCPPair(t)
	defer server.Close()

	prefix := filepath.Join(t.TempDir(), "capture")
	c := &Conn{Prefix: prefix, Conn: client}
	require.NoError(t, c.Initialize())

	defer cleanupPcapng(t, prefix)
	defer c.Close() //nolint:errcheck

	require.Equal(t, client.LocalAddr(), c.LocalAddr())
	require.Equal(t, client.RemoteAddr(), c.RemoteAddr())

	require.NoError(t, c.SetDeadline(time.Now().Add(time.Second)))
	require.NoError(t, c.SetReadDeadline(time.Now().Add(time.Second)))
	require.NoError(t, c.SetWriteDeadline(time.Now().Add(time.Second)))
}
