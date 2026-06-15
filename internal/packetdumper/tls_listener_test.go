package packetdumper

import (
	"crypto/tls"
	"net"
	"path/filepath"
	"testing"

	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestTLSListener(t *testing.T) {
	cert, err := tls.X509KeyPair(test.TLSCertPub, test.TLSCertKey)
	require.NoError(t, err)

	serverTLSConfig := &tls.Config{Certificates: []tls.Certificate{cert}}

	innerLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	prefix := filepath.Join(t.TempDir(), "capture")

	pdLn := &Listener{
		Wrapped: innerLn,
		Prefix:  prefix,
	}

	tlsLn := &TLSListener{
		Wrapped:   pdLn,
		TLSConfig: serverTLSConfig,
	}

	require.Equal(t, innerLn.Addr(), tlsLn.Addr())

	clientDone := make(chan error, 1)
	go func() {
		clientConn, err2 := tls.Dial("tcp", tlsLn.Addr().String(), &tls.Config{InsecureSkipVerify: true}) //nolint:gosec
		if err2 != nil {
			clientDone <- err2
			return
		}
		defer clientConn.Close() //nolint:errcheck

		_, err2 = clientConn.Write([]byte("ping"))
		clientDone <- err2
	}()

	serverConn, err := tlsLn.Accept()
	require.NoError(t, err)
	defer serverConn.Close()

	buf := make([]byte, 4)
	_, err = serverConn.Read(buf)
	require.NoError(t, err)
	require.Equal(t, []byte("ping"), buf)

	require.NoError(t, <-clientDone)

	serverConn.Close()
	tlsLn.Close() //nolint:errcheck

	checkPcapngPresence(t, prefix)
}
