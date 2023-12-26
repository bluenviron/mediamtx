package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPaginate(t *testing.T) {
	items := make([]int, 5)
	for i := 0; i < 5; i++ {
		items[i] = i
	}

	pageCount, err := paginate(&items, "1", "1")
	require.NoError(t, err)
	require.Equal(t, 5, pageCount)
	require.Equal(t, []int{1}, items)

	items = make([]int, 5)
	for i := 0; i < 5; i++ {
		items[i] = i
	}

	pageCount, err = paginate(&items, "3", "2")
	require.NoError(t, err)
	require.Equal(t, 2, pageCount)
	require.Equal(t, []int{}, items)

	items = make([]int, 6)
	for i := 0; i < 6; i++ {
		items[i] = i
	}

	pageCount, err = paginate(&items, "4", "1")
	require.NoError(t, err)
	require.Equal(t, 2, pageCount)
	require.Equal(t, []int{4, 5}, items)
}
