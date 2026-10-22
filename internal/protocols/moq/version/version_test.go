package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	v, ok := Parse("moqt-19")
	require.True(t, ok)
	require.Equal(t, Draft19, v)

	v, ok = Parse("moq-30")
	require.False(t, ok)
	require.Equal(t, Version(0), v)
}
