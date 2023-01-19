package handshake

import (
	"math/rand"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandshake(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:9122")
	require.NoError(t, err)
	defer ln.Close()

	done := make(chan struct{})

	go func() {
		conn, err := ln.Accept()
		require.NoError(t, err)
		defer conn.Close()

		err = DoServer(conn, true)
		require.NoError(t, err)

		close(done)
	}()

	conn, err := net.Dial("tcp", "127.0.0.1:9122")
	require.NoError(t, err)
	defer conn.Close()

	err = DoClient(conn, true)
	require.NoError(t, err)

	<-done
}

// when C1 signature is invalid, S2 must be equal to C1.
func TestHandshakeFallback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:9122")
	require.NoError(t, err)
	defer ln.Close()

	done := make(chan struct{})

	go func() {
		conn, err := ln.Accept()
		require.NoError(t, err)
		defer conn.Close()

		err = DoServer(conn, false)
		require.NoError(t, err)

		close(done)
	}()

	conn, err := net.Dial("tcp", "127.0.0.1:9122")
	require.NoError(t, err)
	defer conn.Close()

	err = C0S0{}.Write(conn)
	require.NoError(t, err)

	c1 := make([]byte, 1536)
	rand.Read(c1[8:])
	_, err = conn.Write(c1)
	require.NoError(t, err)

	err = C0S0{}.Read(conn)
	require.NoError(t, err)

	s1 := C1S1{}
	err = s1.Read(conn, false, false)
	require.NoError(t, err)

	s2 := C2S2{}
	err = s2.Read(conn, false)
	require.NoError(t, err)
	require.Equal(t, c1[8:], s2.Random)

	err = C2S2{
		Time:   s1.Time,
		Random: s1.Random,
		Digest: s1.Digest,
	}.Write(conn)
	require.NoError(t, err)

	<-done
}
