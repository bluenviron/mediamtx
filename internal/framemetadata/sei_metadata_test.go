package framemetadata

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSEIBuildParseH264(t *testing.T) {
	blob := []byte{0x12, 0x34, 0x56}
	sei, err := buildUserDataUnregisteredSEI(true, uuid16, blob)
	require.NoError(t, err)

	got, ok := parseUserDataUnregisteredSEI(true, sei)
	require.True(t, ok)
	require.Equal(t, blob, got)
}

func TestSEIBuildParseH265(t *testing.T) {
	blob := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	sei, err := buildUserDataUnregisteredSEI(false, uuid16, blob)
	require.NoError(t, err)

	got, ok := parseUserDataUnregisteredSEI(false, sei)
	require.True(t, ok)
	require.Equal(t, blob, got)
}

