package mpegts

import (
	"fmt"
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestFromStreamNoSupportedCodecs(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{&format.VP8{}},
	}}}

	r := &stream.Reader{
		Parent: test.Logger(func(logger.Level, string, ...interface{}) {
			t.Error("should not happen")
		}),
	}

	err := FromStream(desc, r, nil, nil, 0)
	require.Equal(t, errNoSupportedCodecs, err)
}

func TestFromStreamSkipUnsupportedTracks(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.H265{}},
		},
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.VP8{}},
		},
	}}

	n := 0

	r := &stream.Reader{
		Parent: test.Logger(func(l logger.Level, format string, args ...interface{}) {
			require.Equal(t, logger.Warn, l)
			if n == 0 {
				require.Equal(t, "skipping track 2 (VP8)", fmt.Sprintf(format, args...))
			}
			n++
		}),
	}

	err := FromStream(desc, r, nil, nil, 0)
	require.NoError(t, err)

	require.Equal(t, 1, n)
}
