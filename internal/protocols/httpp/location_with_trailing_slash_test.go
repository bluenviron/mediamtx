package httpp

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocationWithTrailingSlash(t *testing.T) {
	for _, ca := range []struct {
		name string
		url  *url.URL
		loc  string
	}{
		{
			"with query",
			&url.URL{
				Path:     "/test",
				RawQuery: "key=value",
			},
			"./test/?key=value",
		},
		{
			"xss",
			&url.URL{
				Path: "/www.example.com",
			},
			"./www.example.com/",
		},
		{
			"slashes in path",
			&url.URL{
				Path: "/my/path",
			},
			"./../my/path/",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			require.Equal(t, ca.loc, LocationWithTrailingSlash(ca.url))
		})
	}
}
