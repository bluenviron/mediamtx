package rtmp

import (
	"net/url"
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/stretchr/testify/require"
)

func TestFourCCList(t *testing.T) {
	require.Empty(t, fourCCList(&description.Session{Medias: []*description.Media{{
		Type: description.MediaTypeVideo,
		Formats: []format.Format{&format.H264{
			PayloadTyp: 96,
		}},
	}}}))

	require.NotEmpty(t, fourCCList(&description.Session{Medias: []*description.Media{{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{&format.H265{}},
	}}}))
}

func TestURLWithDefaultPort(t *testing.T) {
	for _, ca := range []struct {
		rawURL string
		host   string
	}{
		{"rtmp://example.com/live/stream", "example.com:1935"},
		{"rtmps://example.com/live/stream", "example.com:443"},
		{"rtmp://example.com:1937/live/stream", "example.com:1937"},
	} {
		t.Run(ca.rawURL, func(t *testing.T) {
			u, err := url.Parse(ca.rawURL)
			require.NoError(t, err)
			require.Equal(t, ca.host, urlWithDefaultPort(u).Host)
		})
	}
}
