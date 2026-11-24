package api_test

import (
	"io"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/defs"
)

func paginationURL(itemsPerPage string, page string) string {
	query := url.Values{"itemsPerPage": {itemsPerPage}, "page": {page}}
	return pathListURL + "?" + query.Encode()
}

func TestPaginate(t *testing.T) {
	for _, ca := range []struct {
		name         string
		itemCount    int
		itemsPerPage string
		page         string
		pageCount    int
		items        []string
	}{
		{"single item per page", 5, "1", "1", 5, []string{"1"}},
		{"last page out of range", 5, "3", "2", 2, []string{}},
		{"partial final page", 6, "4", "1", 2, []string{"4", "5"}},
		{"empty list", 0, "1", "0", 0, []string{}},
	} {
		t.Run(ca.name, func(t *testing.T) {
			names := make([]string, ca.itemCount)
			for i := range names {
				names[i] = strconv.Itoa(i)
			}
			hc := newPathListClient(t, names...)

			var out defs.APIPathList
			httpRequest(t, hc, http.MethodGet, paginationURL(ca.itemsPerPage, ca.page), nil, &out)
			require.Equal(t, ca.itemCount, out.ItemCount)
			require.Equal(t, ca.pageCount, out.PageCount)
			require.NotNil(t, out.Items)
			require.Equal(t, ca.items, pathNames(out.Items))
		})
	}
}

func FuzzPaginate(f *testing.F) {
	hc := newPathListClient(f, "0", "1", "2", "3", "4", "5")
	f.Add("2", "1")

	f.Fuzz(func(t *testing.T, itemsPerPage string, page string) {
		if len(itemsPerPage)+len(page) > 1024 {
			t.Skip()
		}

		res, err := hc.Get(paginationURL(itemsPerPage, page))
		require.NoError(t, err)
		defer res.Body.Close()

		_, err = io.Copy(io.Discard, res.Body)
		require.NoError(t, err)
		require.Contains(t, []int{http.StatusOK, http.StatusBadRequest}, res.StatusCode)
	})
}
