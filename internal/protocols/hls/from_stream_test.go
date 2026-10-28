package hls_test

import (
	"fmt"
	"testing"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/hls"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
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

	err := hls.FromStream(desc, desc, r, m)
	require.Equal(t, hls.ErrNoSupportedCodecs, err)
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

	err := hls.FromStream(desc, desc, r, m)
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

		err := hls.FromStream(desc, desc, r, m)
		require.Equal(t, hls.ErrNoSupportedCodecs, err)
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

		err := hls.FromStream(desc, desc, r, m)
		require.NoError(t, err)
		require.Equal(t, 1, n)
		require.Equal(t, 1, len(m.Tracks))
	})
}

func TestFromStreamMPEGTSVariantSkipUnsupportedAudio(t *testing.T) {
	for _, ca := range []struct {
		name    string
		variant gohlslib.MuxerVariant
		tracks  int
	}{
		{"mpegts", gohlslib.MuxerVariantMPEGTS, 2},
		{"fmp4", gohlslib.MuxerVariantFMP4, 3},
	} {
		t.Run(ca.name, func(t *testing.T) {
			desc := &description.Session{Medias: []*description.Media{
				test.MediaH264,
				{
					Type:    description.MediaTypeAudio,
					Formats: []format.Format{&format.Opus{PayloadTyp: 96, ChannelCount: 2}},
				},
				test.MediaMPEG4Audio,
			}}

			n := 0

			r := &stream.Reader{
				Parent: test.Logger(func(l logger.Level, f string, args ...any) {
					require.Equal(t, logger.Warn, l)
					require.Equal(t, "skipping track 2 (Opus)", fmt.Sprintf(f, args...))
					n++
				}),
			}

			m := &gohlslib.Muxer{Variant: ca.variant}

			err := hls.FromStream(desc, desc, r, m)
			require.NoError(t, err)
			require.Equal(t, ca.tracks, len(m.Tracks))

			if ca.variant == gohlslib.MuxerVariantMPEGTS {
				require.Equal(t, 1, n)
			} else {
				require.Equal(t, 0, n)
			}

			err = m.Start()
			require.NoError(t, err)
			m.Close()
		})
	}
}

func TestFromStreamMPEGTSVariantNoSupportedAudio(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{{
		Type:    description.MediaTypeAudio,
		Formats: []format.Format{&format.Opus{PayloadTyp: 96, ChannelCount: 2}},
	}}}

	r := &stream.Reader{
		Parent: test.Logger(func(logger.Level, string, ...any) {
			t.Error("should not happen")
		}),
	}

	m := &gohlslib.Muxer{Variant: gohlslib.MuxerVariantMPEGTS}

	err := hls.FromStream(desc, desc, r, m)
	require.Equal(t, hls.ErrNoSupportedCodecs, err)
}

func TestFromStreamMPEGTSVariantSkipExtraAudio(t *testing.T) {
	for _, ca := range []struct {
		name    string
		variant gohlslib.MuxerVariant
		tracks  int
	}{
		{"mpegts", gohlslib.MuxerVariantMPEGTS, 2},
		{"fmp4", gohlslib.MuxerVariantFMP4, 3},
	} {
		t.Run(ca.name, func(t *testing.T) {
			desc := &description.Session{Medias: []*description.Media{
				test.MediaH264,
				test.MediaMPEG4Audio,
				{
					Type: description.MediaTypeAudio,
					Formats: []format.Format{&format.MPEG4Audio{
						PayloadTyp:       96,
						Config:           test.FormatMPEG4Audio.Config,
						SizeLength:       13,
						IndexLength:      3,
						IndexDeltaLength: 3,
					}},
				},
			}}

			n := 0

			r := &stream.Reader{
				Parent: test.Logger(func(l logger.Level, f string, args ...any) {
					require.Equal(t, logger.Warn, l)
					require.Equal(t, "skipping track 3 (MPEG-4 Audio)", fmt.Sprintf(f, args...))
					n++
				}),
			}

			m := &gohlslib.Muxer{Variant: ca.variant}

			err := hls.FromStream(desc, desc, r, m)
			require.NoError(t, err)
			require.Equal(t, ca.tracks, len(m.Tracks))

			if ca.variant == gohlslib.MuxerVariantMPEGTS {
				require.Equal(t, 1, n)
			} else {
				require.Equal(t, 0, n)
			}

			err = m.Start()
			require.NoError(t, err)
			m.Close()
		})
	}
}
