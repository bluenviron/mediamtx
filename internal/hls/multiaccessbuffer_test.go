package hls

import (
	"io"
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

	buf = make([]byte, 10)
	n, err = r.Read(buf)
	require.NoError(t, err)
	require.Equal(t, []byte{0x05, 0x06, 0x07, 0x08}, buf[:n])

	m.Write([]byte{0x09, 0x0a, 0x0b, 0x0c})

	m.Close()

	buf = make([]byte, 10)
	n, err = r.Read(buf)
	require.NoError(t, err)
	require.Equal(t, []byte{0x09, 0x0a, 0x0b, 0x0c}, buf[:n])

	buf = make([]byte, 10)
	_, err = r.Read(buf)
	require.Equal(t, io.EOF, err)
}
