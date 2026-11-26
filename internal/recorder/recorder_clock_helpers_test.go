package recorder

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func clockEvidenceDir(t *testing.T) string {
	t.Helper()
	base := os.Getenv("MTX_CLOCK_EVIDENCE")
	if base == "" {
		return t.TempDir()
	}
	base = filepath.Join(base, t.Name())
	require.NoError(t, os.MkdirAll(base, 0o755))
	dir, err := os.MkdirTemp(base, "run-")
	require.NoError(t, err)
	t.Logf("EVIDENCE %s", dir)
	return dir
}

func clockPayload(id uint32, size int) []byte {
	p := make([]byte, size)
	for j := range p {
		p[j] = byte(id) ^ byte(j)
	}
	binary.BigEndian.PutUint32(p, id)
	return p
}
