package writebuffer_test

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/writebuffer"
)

func TestSetWriteBuffer(t *testing.T) {
	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)
	defer pc.Close() //nolint:errcheck

	err = writebuffer.SetWriteBuffer(pc, 10000)
	require.NoError(t, err)
}
