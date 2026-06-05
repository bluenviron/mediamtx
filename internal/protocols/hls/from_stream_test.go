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
		Parent: test.Logger(func(logger.Level, string, ...any) {
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
		Parent: test.Logger(func(l logger.Level, format string, args ...any) {
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

func TestFromStreamKLVRequiresMPEGTSVariant(t *testing.T) {
	t.Run("klv only, non-mpegts variant", func(t *testing.T) {
		desc := &description.Session{Medias: []*description.Media{{
			Type:    description.MediaTypeApplication,
			Formats: []format.Format{&format.KLV{PayloadTyp: 96}},
		}}}

		r := &stream.Reader{
			Parent: test.Logger(func(logger.Level, string, ...any) {
				t.Error("should not happen")
			}),
		}

		m := &gohlslib.Muxer{Variant: gohlslib.MuxerVariantFMP4}

		err := FromStream(desc, r, m)
		require.Equal(t, ErrNoSupportedCodecs, err)
	})

	t.Run("klv alongside video, non-mpegts variant", func(t *testing.T) {
		desc := &description.Session{Medias: []*description.Media{
			{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{&format.H264{PayloadTyp: 96, PacketizationMode: 1}},
			},
			{
				Type:    description.MediaTypeApplication,
				Formats: []format.Format{&format.KLV{PayloadTyp: 97}},
			},
		}}

		n := 0

		r := &stream.Reader{
			Parent: test.Logger(func(l logger.Level, f string, args ...any) {
				require.Equal(t, logger.Warn, l)
				require.Equal(t, "skipping track 2 (KLV)", fmt.Sprintf(f, args...))
				n++
			}),
		}

		m := &gohlslib.Muxer{Variant: gohlslib.MuxerVariantFMP4}

		err := FromStream(desc, r, m)
		require.NoError(t, err)
		require.Equal(t, 1, n)
		require.Equal(t, 1, len(m.Tracks))
	})
}
