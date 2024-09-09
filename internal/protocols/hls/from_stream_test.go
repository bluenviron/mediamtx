package hls

import (
	"fmt"
	"testing"

	"github.com/bluenviron/gohlslib"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediamtx/internal/asyncwriter"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestFromStreamNoSupportedCodecs(t *testing.T) {
	stream, err := stream.New(
		1460,
		&description.Session{Medias: []*description.Media{{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.VP8{}},
		}}},
		true,
		test.NilLogger,
	)
	require.NoError(t, err)

	writer := asyncwriter.New(0, nil)

	l := test.Logger(func(logger.Level, string, ...interface{}) {
		t.Error("should not happen")
	})

	err = FromStream(stream, writer, nil, l)
	require.Equal(t, ErrNoSupportedCodecs, err)
}

func TestFromStreamSkipUnsupportedTracks(t *testing.T) {
	stream, err := stream.New(
		1460,
		&description.Session{Medias: []*description.Media{
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
		}},
		true,
		test.NilLogger,
	)
	require.NoError(t, err)

	writer := asyncwriter.New(0, nil)

	m := &gohlslib.Muxer{}

	n := 0

	l := test.Logger(func(l logger.Level, format string, args ...interface{}) {
		require.Equal(t, logger.Warn, l)
		switch n {
		case 0:
			require.Equal(t, "skipping track with codec VP8", fmt.Sprintf(format, args...))
		case 1:
			require.Equal(t, "skipping track with codec MPEG-1/2 Audio", fmt.Sprintf(format, args...))
		}
		n++
	})

	err = FromStream(stream, writer, m, l)
	require.NoError(t, err)
	require.Equal(t, 2, n)
}
