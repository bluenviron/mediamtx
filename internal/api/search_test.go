package api //nolint:revive

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSearch(t *testing.T) {
	items := []string{"Cam1", "cam2", "door"}
	identity := func(s string) string { return s }

	require.Equal(t, items, search(items, "", identity))
	require.Equal(t, []string{"Cam1", "cam2"}, search(items, "CAM", identity))
	require.Equal(t, []string{"door"}, search(items, "oo", identity))
	require.Empty(t, search(items, "xyz", identity))
}
