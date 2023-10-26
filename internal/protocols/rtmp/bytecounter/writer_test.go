package bytecounter

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriter(t *testing.T) {
	var buf bytes.Buffer

	w := NewWriter(&buf)
	w.SetCount(100)

	_, err := w.Write(bytes.Repeat([]byte{0x01}, 64))
	require.NoError(t, err)
	require.Equal(t, uint64(100+64), w.Count())
}
