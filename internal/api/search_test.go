package api_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/defs"
)

func TestSearch(t *testing.T) {
	hc := newPathListClient(t, "Cam1", "cam2", "door")

	for _, ca := range []struct {
		name  string
		query string
		items []string
	}{
		{"empty query", "", []string{"Cam1", "cam2", "door"}},
		{"case insensitive", "CAM", []string{"Cam1", "cam2"}},
		{"substring", "oo", []string{"door"}},
		{"no matches", "xyz", []string{}},
	} {
		t.Run(ca.name, func(t *testing.T) {
			query := url.Values{"search": {ca.query}}
			var out defs.APIPathList
			httpRequest(t, hc, http.MethodGet, pathListURL+"?"+query.Encode(), nil, &out)
			require.Equal(t, len(ca.items), out.ItemCount)
			require.Equal(t, ca.items, pathNames(out.Items))
		})
	}
}
