package formatlabel_test

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/formatlabel"
)

func TestFormatToLabel(t *testing.T) {
	codecs := []formatlabel.Label{
		formatlabel.FormatToLabel(&format.AV1{}),
		formatlabel.FormatToLabel(&format.VP9{}),
		formatlabel.FormatToLabel(&format.VP8{}),
		formatlabel.FormatToLabel(&format.H265{}),
		formatlabel.FormatToLabel(&format.H264{}),
		formatlabel.FormatToLabel(&format.MPEG4Video{}),
		formatlabel.FormatToLabel(&format.MPEG1Video{}),
		formatlabel.FormatToLabel(&format.MJPEG{}),
		formatlabel.FormatToLabel(&format.Opus{}),
		formatlabel.FormatToLabel(&format.Generic{RTPMa: "FLAC/44100/2"}),
		formatlabel.FormatToLabel(&format.Vorbis{}),
		formatlabel.FormatToLabel(&format.MPEG4Audio{}),
		formatlabel.FormatToLabel(&format.MPEG4AudioLATM{}),
		formatlabel.FormatToLabel(&format.MPEG1Audio{}),
		formatlabel.FormatToLabel(&format.AC3{}),
		formatlabel.FormatToLabel(&format.Speex{}),
		formatlabel.FormatToLabel(&format.G726{}),
		formatlabel.FormatToLabel(&format.G722{}),
		formatlabel.FormatToLabel(&format.G711{}),
		formatlabel.FormatToLabel(&format.LPCM{}),
		formatlabel.FormatToLabel(&format.MPEGTS{}),
		formatlabel.FormatToLabel(&format.KLV{}),
		formatlabel.FormatToLabel(&format.Generic{}),
	}

	require.Equal(t, []formatlabel.Label{
		formatlabel.AV1,
		formatlabel.VP9,
		formatlabel.VP8,
		formatlabel.H265,
		formatlabel.H264,
		formatlabel.MPEG4Video,
		formatlabel.MPEG1Video,
		formatlabel.MJPEG,
		formatlabel.Opus,
		formatlabel.FLAC,
		formatlabel.Vorbis,
		formatlabel.MPEG4Audio,
		formatlabel.MPEG4AudioLATM,
		formatlabel.MPEG1Audio,
		formatlabel.AC3,
		formatlabel.Speex,
		formatlabel.G726,
		formatlabel.G722,
		formatlabel.G711,
		formatlabel.LPCM,
		formatlabel.MPEGTS,
		formatlabel.KLV,
		formatlabel.Generic,
	}, codecs)
}

func TestFormatsToLabels(t *testing.T) {
	require.Equal(t, []formatlabel.Label{
		formatlabel.H264,
		formatlabel.Opus,
		formatlabel.KLV,
		formatlabel.Generic,
	}, formatlabel.FormatsToLabels([]format.Format{
		&format.H264{},
		&format.Opus{},
		&format.KLV{},
		&format.Generic{},
	}))
}

func TestMediasToLabels(t *testing.T) {
	require.Equal(t, []formatlabel.Label{
		formatlabel.H264,
		formatlabel.Opus,
		formatlabel.G711,
		formatlabel.KLV,
	}, formatlabel.MediasToLabels([]*description.Media{
		{Formats: []format.Format{&format.H264{}}},
		{Formats: []format.Format{&format.Opus{}, &format.G711{}}},
		{Formats: []format.Format{&format.KLV{}}},
	}))
}
