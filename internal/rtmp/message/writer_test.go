package message

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/bytecounter"
)

func TestWriter(t *testing.T) {
	for _, ca := range readWriterCases {
		t.Run(ca.name, func(t *testing.T) {
			var buf bytes.Buffer
			r := NewWriter(bytecounter.NewWriter(&buf), true)
			err := r.Write(ca.dec)
			require.NoError(t, err)
			require.Equal(t, ca.enc, buf.Bytes())
		})
	}
}
