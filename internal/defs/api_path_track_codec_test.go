package defs

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T {
	return &v
}

func testFormatH264() *format.H264 {
	return &format.H264{
		PayloadTyp: 96,
		SPS: []byte{
			0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
			0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
			0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
		},
		PPS:               []byte{0x08, 0x06, 0x07, 0x08},
		PacketizationMode: 1,
	}
}

func testFormatMPEG4Audio() *format.MPEG4Audio {
	return &format.MPEG4Audio{
		PayloadTyp: 96,
		Config: &mpeg4audio.AudioSpecificConfig{
			Type:          2,
			SampleRate:    44100,
			ChannelCount:  2,
			ChannelConfig: 2,
		},
		SizeLength:       13,
		IndexLength:      3,
		IndexDeltaLength: 3,
	}
}

func testFormatH265() *format.H265 {
	return &format.H265{
		PayloadTyp: 96,
		VPS: []byte{
			0x40, 0x01, 0x0c, 0x01, 0xff, 0xff, 0x02, 0x20,
			0x00, 0x00, 0x03, 0x00, 0xb0, 0x00, 0x00, 0x03,
			0x00, 0x00, 0x03, 0x00, 0x7b, 0x18, 0xb0, 0x24,
		},
		SPS: []byte{
			0x42, 0x01, 0x01, 0x02, 0x20, 0x00, 0x00, 0x03,
			0x00, 0xb0, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03,
			0x00, 0x7b, 0xa0, 0x07, 0x82, 0x00, 0x88, 0x7d,
			0xb6, 0x71, 0x8b, 0x92, 0x44, 0x80, 0x53, 0x88,
			0x88, 0x92, 0xcf, 0x24, 0xa6, 0x92, 0x72, 0xc9,
			0x12, 0x49, 0x22, 0xdc, 0x91, 0xaa, 0x48, 0xfc,
			0xa2, 0x23, 0xff, 0x00, 0x01, 0x00, 0x01, 0x6a,
			0x02, 0x02, 0x02, 0x01,
		},
		PPS: []byte{
			0x44, 0x01, 0xc0, 0x25, 0x2f, 0x05, 0x32, 0x40,
		},
	}
}

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

func TestMediasToTracks(t *testing.T) {
	tracks := MediasToTracks([]*description.Media{
		{Formats: []format.Format{testFormatH264()}},
		{Formats: []format.Format{testFormatH265()}},
		{
			Formats: []format.Format{
				&format.AV1{Profile: ptr(1), LevelIdx: ptr(9), Tier: ptr(0)},
				&format.VP9{ProfileID: ptr(2)},
				&format.MJPEG{},
				&format.Opus{ChannelCount: 2},
				&format.G711{MULaw: true, SampleRate: 8000, ChannelCount: 1},
				&format.LPCM{BitDepth: 16, SampleRate: 48000, ChannelCount: 2},
			},
		},
		{
			Formats: []format.Format{testFormatMPEG4Audio()},
		},
	})

	require.Equal(t, []APIPathTrack{
		{
			Codec: APIPathTrackCodecH264,
			CodecProps: &APIPathTrackCodecPropsH264{
				Width:   1920,
				Height:  1080,
				Profile: "Baseline",
				Level:   "4",
			},
		},
		{
			Codec: APIPathTrackCodecH265,
			CodecProps: &APIPathTrackCodecPropsH265{
				Width:   960,
				Height:  540,
				Profile: "Main 10",
				Level:   "4.1",
			},
		},
		{
			Codec: APIPathTrackCodecAV1,
			CodecProps: &APIPathTrackCodecPropsAV1{
				Profile: 1,
				Level:   9,
				Tier:    0,
			},
		},
		{
			Codec: APIPathTrackCodecVP9,
			CodecProps: &APIPathTrackCodecPropsVP9{
				Profile: 2,
			},
		},
		{
			Codec:      APIPathTrackCodecMJPEG,
			CodecProps: nil,
		},
		{
			Codec: APIPathTrackCodecOpus,
			CodecProps: &APIPathTrackCodecPropsOpus{
				ChannelCount: 2,
			},
		},
		{
			Codec: APIPathTrackCodecG711,
			CodecProps: &APIPathTrackCodecPropsG711{
				MULaw:        true,
				SampleRate:   8000,
				ChannelCount: 1,
			},
		},
		{
			Codec: APIPathTrackCodecLPCM,
			CodecProps: &APIPathTrackCodecPropsLPCM{
				BitDepth:     16,
				SampleRate:   48000,
				ChannelCount: 2,
			},
		},
		{
			Codec: APIPathTrackCodecMPEG4Audio,
			CodecProps: &APIPathTrackCodecPropsMPEG4Audio{
				SampleRate:   44100,
				ChannelCount: 2,
			},
		},
	}, tracks)
}
