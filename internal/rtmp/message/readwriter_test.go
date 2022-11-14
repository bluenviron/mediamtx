package message

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/bytecounter"
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

	rw1 := NewReadWriter(bytecounter.NewReadWriter(&duplexRW{
		Reader: &buf2,
		Writer: &buf1,
	}), true)
	err := rw1.Write(&MsgAcknowledge{
		Value: 7863534,
	})
	require.NoError(t, err)

	rw2 := NewReadWriter(bytecounter.NewReadWriter(&duplexRW{
		Reader: &buf1,
		Writer: &buf2,
	}), true)
	_, err = rw2.Read()
	require.NoError(t, err)
}

func TestReadWriterPing(t *testing.T) {
	var buf1 bytes.Buffer
	var buf2 bytes.Buffer

	rw1 := NewReadWriter(bytecounter.NewReadWriter(&duplexRW{
		Reader: &buf2,
		Writer: &buf1,
	}), true)
	err := rw1.Write(&MsgUserControlPingRequest{
		ServerTime: 143424312,
	})
	require.NoError(t, err)

	rw2 := NewReadWriter(bytecounter.NewReadWriter(&duplexRW{
		Reader: &buf1,
		Writer: &buf2,
	}), true)
	_, err = rw2.Read()
	require.NoError(t, err)

	msg, err := rw1.Read()
	require.NoError(t, err)
	require.Equal(t, &MsgUserControlPingResponse{
		ServerTime: 143424312,
	}, msg)
}
