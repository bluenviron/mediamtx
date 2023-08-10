package message

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/rtmp/bytecounter"
)

type duplexRW struct {
	io.Reader
	io.Writer
}

func (d *duplexRW) Read(p []byte) (int, error) {
	return d.Reader.Read(p)
}

func (d *duplexRW) Write(p []byte) (int, error) {
	return d.Writer.Write(p)
}

func TestReadWriterAcknowledge(t *testing.T) {
	var buf1 bytes.Buffer
	var buf2 bytes.Buffer

	bc1 := bytecounter.NewReadWriter(&duplexRW{
		Reader: &buf2,
		Writer: &buf1,
	})
	rw1 := NewReadWriter(bc1, bc1, true)
	err := rw1.Write(&Acknowledge{
		Value: 7863534,
	})
	require.NoError(t, err)

	bc2 := bytecounter.NewReadWriter(&duplexRW{
		Reader: &buf1,
		Writer: &buf2,
	})
	rw2 := NewReadWriter(bc2, bc2, true)
	_, err = rw2.Read()
	require.NoError(t, err)
}

func TestReadWriterPing(t *testing.T) {
	var buf1 bytes.Buffer
	var buf2 bytes.Buffer

	bc1 := bytecounter.NewReadWriter(&duplexRW{
		Reader: &buf2,
		Writer: &buf1,
	})
	rw1 := NewReadWriter(bc1, bc1, true)
	err := rw1.Write(&UserControlPingRequest{
		ServerTime: 143424312,
	})
	require.NoError(t, err)

	bc2 := bytecounter.NewReadWriter(&duplexRW{
		Reader: &buf1,
		Writer: &buf2,
	})
	rw2 := NewReadWriter(bc2, bc2, true)
	_, err = rw2.Read()
	require.NoError(t, err)

	msg, err := rw1.Read()
	require.NoError(t, err)
	require.Equal(t, &UserControlPingResponse{
		ServerTime: 143424312,
	}, msg)
}
