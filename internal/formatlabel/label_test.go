package formatlabel

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/stretchr/testify/require"
)

func TestFormatToLabel(t *testing.T) {
	codecs := []Label{
		FormatToLabel(&format.AV1{}),
		FormatToLabel(&format.VP9{}),
		FormatToLabel(&format.VP8{}),
		FormatToLabel(&format.H265{}),
		FormatToLabel(&format.H264{}),
		FormatToLabel(&format.MPEG4Video{}),
		FormatToLabel(&format.MPEG1Video{}),
		FormatToLabel(&format.MJPEG{}),
		FormatToLabel(&format.Opus{}),
		FormatToLabel(&format.Vorbis{}),
		FormatToLabel(&format.MPEG4Audio{}),
		FormatToLabel(&format.MPEG4AudioLATM{}),
		FormatToLabel(&format.MPEG1Audio{}),
		FormatToLabel(&format.AC3{}),
		FormatToLabel(&format.Speex{}),
		FormatToLabel(&format.G726{}),
		FormatToLabel(&format.G722{}),
		FormatToLabel(&format.G711{}),
		FormatToLabel(&format.LPCM{}),
		FormatToLabel(&format.MPEGTS{}),
		FormatToLabel(&format.KLV{}),
		FormatToLabel(&format.Generic{}),
	}

	require.Equal(t, []Label{
		AV1,
		VP9,
		VP8,
		H265,
		H264,
		MPEG4Video,
		MPEG1Video,
		MJPEG,
		Opus,
		Vorbis,
		MPEG4Audio,
		MPEG4AudioLATM,
		MPEG1Audio,
		AC3,
		Speex,
		G726,
		G722,
		G711,
		LPCM,
		MPEGTS,
		KLV,
		Generic,
	}, codecs)
}

func TestFormatsToLabels(t *testing.T) {
	require.Equal(t, []Label{
		H264,
		Opus,
		KLV,
		Generic,
	}, FormatsToLabels([]format.Format{
		&format.H264{},
		&format.Opus{},
		&format.KLV{},
		&format.Generic{},
	}))
}

func TestMediasToLabels(t *testing.T) {
	require.Equal(t, []Label{
		H264,
		Opus,
		G711,
		KLV,
	}, MediasToLabels([]*description.Media{
		{Formats: []format.Format{&format.H264{}}},
		{Formats: []format.Format{&format.Opus{}, &format.G711{}}},
		{Formats: []format.Format{&format.KLV{}}},
	}))
}
