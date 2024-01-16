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

func TestPathToUnparametrizedRoot(t *testing.T) {
	fromRoot := []string{"/root/directory/%%param", "/root/directory"}
	relative := []string{"./root/directory/%%param", "./root/directory"}
	fromRootNoParam := []string{"/root/directory", "/root/directory"}
	relativeNoParam := []string{"./root/directory", "./root/directory"}
	fromRootMultipleParams := []string{"/root/directory/%%param2/%%param3", "/root/directory"}
	relativeMultipleParam := []string{"./root/directory/%%param/%%param2/%%param3", "./root/directory"}

	require.Equal(t, PathToUnparametrizedRoot(fromRoot[0]), fromRoot[1])
	require.Equal(t, PathToUnparametrizedRoot(relative[0]), relative[1])
	require.Equal(t, PathToUnparametrizedRoot(fromRootNoParam[0]), fromRootNoParam[1])
	require.Equal(t, PathToUnparametrizedRoot(relativeNoParam[0]), relativeNoParam[1])
	require.Equal(t, PathToUnparametrizedRoot(fromRootMultipleParams[0]), fromRootMultipleParams[1])
	require.Equal(t, PathToUnparametrizedRoot(relativeMultipleParam[0]), relativeMultipleParam[1])
}

func TestIsPathSafe(t *testing.T) {
	safePath := "simple/path/to/recording.mp4"
	unSafePath := "../../sh"
	unSafePathFromRoot := "/../../sh"
	unSafePathRelative := "./../../sh"

	require.True(t, isPathSafe(safePath))
	require.False(t, isPathSafe(unSafePath))
	require.False(t, isPathSafe(unSafePathFromRoot))
	require.False(t, isPathSafe(unSafePathRelative))
}
