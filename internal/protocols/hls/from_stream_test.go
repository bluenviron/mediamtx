package hls

import (
	"fmt"
	"testing"

	"github.com/bluenviron/gohlslib/v2"
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

	m := &gohlslib.Muxer{}

	err := FromStream(desc, r, m)
	require.Equal(t, ErrNoSupportedCodecs, err)
}

func TestFromStreamSkipUnsupportedTracks(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.VP9{}},
		},
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.VP8{}},
		},
		{
			Type:    description.MediaTypeAudio,
			Formats: []format.Format{&format.MPEG1Audio{}},
		},
	}}

	m := &gohlslib.Muxer{}

	n := 0

	r := &stream.Reader{
		Parent: test.Logger(func(l logger.Level, format string, args ...interface{}) {
			require.Equal(t, logger.Warn, l)
			switch n {
			case 0:
				require.Equal(t, "skipping track 2 (VP8)", fmt.Sprintf(format, args...))
			case 1:
				require.Equal(t, "skipping track 3 (MPEG-1/2 Audio)", fmt.Sprintf(format, args...))
			}
			n++
		}),
	}

	err := FromStream(desc, r, m)
	require.NoError(t, err)

	require.Equal(t, 2, n)
}
