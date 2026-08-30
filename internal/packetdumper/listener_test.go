package packetdumper

import (
	"net"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListener(t *testing.T) {
	innerLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	prefix := filepath.Join(t.TempDir(), "capture")
	ln := &Listener{
		Wrapped: innerLn,
		Prefix:  prefix,
	}

	require.Equal(t, innerLn.Addr(), ln.Addr())

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	defer clientConn.Close()

	serverConn, err := ln.Accept()
	require.NoError(t, err)
	defer serverConn.Close()

	_, err = clientConn.Write([]byte("ping"))
	require.NoError(t, err)

	buf := make([]byte, 4)
	_, err = serverConn.Read(buf)
	require.NoError(t, err)
	require.Equal(t, []byte("ping"), buf)

	serverConn.Close()
	clientConn.Close()
	ln.Close() //nolint:errcheck

	checkPcapngPresence(t, prefix)
}
