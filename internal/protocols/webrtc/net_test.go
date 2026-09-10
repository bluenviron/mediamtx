package webrtc

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNetListenUDP(t *testing.T) {
	n := &Net{
		UDPReadBufferSize:  106496,
		UDPWriteBufferSize: 106496,
	}

	conn, err := n.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck
}
