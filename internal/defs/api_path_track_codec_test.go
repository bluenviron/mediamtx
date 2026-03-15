package defs

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/stretchr/testify/require"
)

func TestFormatsToCodecs(t *testing.T) {
	codecs := FormatsToCodecs([]format.Format{
		&format.AV1{},
		&format.VP9{},
		&format.VP8{},
		&format.H265{},
		&format.H264{},
		&format.MPEG4Video{},
		&format.MPEG1Video{},
		&format.MJPEG{},
		&format.Opus{},
		&format.Vorbis{},
		&format.MPEG4Audio{},
		&format.MPEG4AudioLATM{},
		&format.MPEG1Audio{},
		&format.AC3{},
		&format.Speex{},
		&format.G726{},
		&format.G722{},
		&format.G711{},
		&format.LPCM{},
		&format.MPEGTS{},
		&format.KLV{},
		&format.Generic{},
	})

	require.Equal(t, []APIPathTrackCodec{
		APIPathTrackCodecAV1,
		APIPathTrackCodecVP9,
		APIPathTrackCodecVP8,
		APIPathTrackCodecH265,
		APIPathTrackCodecH264,
		APIPathTrackCodecMPEG4Video,
		APIPathTrackCodecMPEG1Video,
		APIPathTrackCodecMJPEG,
		APIPathTrackCodecOpus,
		APIPathTrackCodecVorbis,
		APIPathTrackCodecMPEG4Audio,
		APIPathTrackCodecMPEG4AudioLATM,
		APIPathTrackCodecMPEG1Audio,
		APIPathTrackCodecAC3,
		APIPathTrackCodecSpeex,
		APIPathTrackCodecG726,
		APIPathTrackCodecG722,
		APIPathTrackCodecG711,
		APIPathTrackCodecLPCM,
		APIPathTrackCodecMPEGTS,
		APIPathTrackCodecKLV,
		APIPathTrackCodecGeneric,
	}, codecs)
}
