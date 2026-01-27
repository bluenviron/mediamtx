package conf

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/stretchr/testify/require"
)

func TestPathClone(t *testing.T) {
	original := &Path{
		Name:                "example",
		RTSPTransport:       RTSPTransport{ptrOf(gortsplib.ProtocolUDP)},
		SourceAnyPortEnable: ptrOf(true),
		RecordPath:          "/var/recordings",
	}

	clone := original.Clone()
	require.Equal(t, original, clone)
}
