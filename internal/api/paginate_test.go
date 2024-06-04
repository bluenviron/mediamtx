package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPaginate(t *testing.T) {
	func() {
		items := make([]int, 5)
		for i := 0; i < 5; i++ {
			items[i] = i
		}

		pageCount, err := paginate(&items, "1", "1")
		require.NoError(t, err)
		require.Equal(t, 5, pageCount)
		require.Equal(t, []int{1}, items)
	}()

	func() {
		items := make([]int, 5)
		for i := 0; i < 5; i++ {
			items[i] = i
		}

		pageCount, err := paginate(&items, "3", "2")
		require.NoError(t, err)
		require.Equal(t, 2, pageCount)
		require.Equal(t, []int{}, items)
	}()

	func() {
		items := make([]int, 6)
		for i := 0; i < 6; i++ {
			items[i] = i
		}

		pageCount, err := paginate(&items, "4", "1")
		require.NoError(t, err)
		require.Equal(t, 2, pageCount)
		require.Equal(t, []int{4, 5}, items)
	}()

	func() {
		items := make([]int, 0)

		pageCount, err := paginate(&items, "1", "0")
		require.NoError(t, err)
		require.Equal(t, 0, pageCount)
		require.Equal(t, []int{}, items)
	}()
}

func FuzzPaginate(f *testing.F) {
	f.Fuzz(func(_ *testing.T, str1 string, str2 string) {
		items := make([]int, 6)
		for i := 0; i < 6; i++ {
			items[i] = i
		}

		paginate(&items, str1, str2) //nolint:errcheck
	})
}
