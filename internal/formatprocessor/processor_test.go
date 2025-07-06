package formatprocessor

import (
	"testing"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	for _, ca := range []struct {
		name string
		in   format.Format
		out  Processor
	}{
		{
			"av1",
			&format.AV1{},
			&av1{},
		},
		{
			"vp9",
			&format.VP9{},
			&vp9{},
		},
		{
			"vp8",
			&format.VP8{},
			&vp8{},
		},
		{
			"h265",
			&format.H265{},
			&h265{},
		},
		{
			"h264",
			&format.H264{},
			&h264{},
		},
		{
			"mpeg4 video",
			&format.MPEG4Video{},
			&mpeg4Video{},
		},
		{
			"mpeg1 video",
			&format.MPEG1Video{},
			&mpeg1Video{},
		},
		{
			"mpeg1 mjpeg",
			&format.MPEG1Audio{},
			&mpeg1Audio{},
		},
		{
			"opus",
			&format.Opus{},
			&opus{},
		},
		{
			"mpeg4 audio",
			&format.MPEG4Audio{},
			&mpeg4Audio{},
		},
		{
			"mpeg1 audio",
			&format.MPEG1Audio{},
			&mpeg1Audio{},
		},
		{
			"ac3",
			&format.AC3{},
			&ac3{},
		},
		{
			"g711",
			&format.G711{},
			&g711{},
		},
		{
			"lpcm",
			&format.LPCM{},
			&lpcm{},
		},
		{
			"klv",
			&format.KLV{
				PayloadTyp: 96,
			},
			&klv{},
		},
		{
			"generic",
			&format.Generic{},
			&generic{},
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			p, err := New(1450, ca.in, false, nil)
			require.NoError(t, err)
			require.IsType(t, ca.out, p)
		})
	}
}
