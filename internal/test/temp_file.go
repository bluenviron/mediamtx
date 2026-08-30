package test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// CreateTempFile creates a temporary file with given content.
func CreateTempFile(t *testing.T, byts []byte) string {
	tmpf, err := os.CreateTemp(t.TempDir(), "rtsp-")
	require.NoError(t, err)
	defer tmpf.Close()

	_, err = tmpf.Write(byts)
	require.NoError(t, err)

	return tmpf.Name()
}
