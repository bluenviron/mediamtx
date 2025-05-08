package bytecounter

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReader(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(bytes.Repeat([]byte{0x01}, 1024))

	r := NewReader(&buf)
	r.SetCount(100)

	buf2 := make([]byte, 64)
	n, err := r.Read(buf2)
	require.NoError(t, err)
	require.Equal(t, 64, n)

	require.Equal(t, uint64(100+64), r.Count())
}
