package clienthls

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMultiAccessBuffer(t *testing.T) {
	m := newMultiAccessBuffer()

	m.Write([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08})

	r := m.NewReader()

	buf := make([]byte, 4)
	n, err := r.Read(buf)
	require.NoError(t, err)
	require.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, buf[:n])

	m.Close()

	buf = make([]byte, 10)
	n, err = r.Read(buf)
	require.NoError(t, err)
	require.Equal(t, []byte{0x05, 0x06, 0x07, 0x08}, buf[:n])
}
