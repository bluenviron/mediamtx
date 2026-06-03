package moq

import (
	"encoding/base64"
	"encoding/hex"
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/flac"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/stretchr/testify/require"
)

func TestFromStream(t *testing.T) {
	flacEnc, err := (&flac.StreamInfo{
		SampleRate:   44100,
		ChannelCount: 2,
		BitDepth:     16,
	}).Marshal()
	require.NoError(t, err)

	mpeg4Config := &mpeg4audio.AudioSpecificConfig{
		Type:          2,
		SampleRate:    44100,
		ChannelConfig: 2,
	}
	mpeg4Enc, err := mpeg4Config.Marshal()
	require.NoError(t, err)

	desc := &description.Session{
		Medias: []*description.Media{
			{Formats: []format.Format{&format.AV1{}}},
			{Formats: []format.Format{&format.VP9{}}},
			{Formats: []format.Format{&format.VP8{}}},
			{Formats: []format.Format{&format.H265{}}},
			{Formats: []format.Format{&format.H264{}}},
			{Formats: []format.Format{&format.Opus{ChannelCount: 2}}},
			{Formats: []format.Format{&format.Generic{
				RTPMa:    "FLAC/44100/2",
				ClockRat: 44100,
				FMT:      map[string]string{"streaminfo": hex.EncodeToString(flacEnc)},
			}}},
			{Formats: []format.Format{&format.MPEG4Audio{
				Config:           mpeg4Config,
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			}}},
			{Formats: []format.Format{&format.G711{
				SampleRate:   8000,
				ChannelCount: 1,
			}}},
			{Formats: []format.Format{&format.LPCM{
				BitDepth:     16,
				SampleRate:   44100,
				ChannelCount: 2,
			}}},
		},
	}

	tracks, err := FromStream(desc)
	require.NoError(t, err)

	for _, track := range tracks {
		track.OnData = nil
	}

	require.Equal(t, []*Track{
		{
			Codec: "av01.0.04M.08", Media: desc.Medias[0],
			Format: desc.Medias[0].Formats[0],
		},
		{
			Codec: "vp09.00.10.08", Media: desc.Medias[1],
			Format: desc.Medias[1].Formats[0],
		},
		{
			Codec: "vp8", Media: desc.Medias[2],
			Format: desc.Medias[2].Formats[0],
		},
		{
			Codec: "hev1.1.6.L93.B0", Media: desc.Medias[3],
			Format: desc.Medias[3].Formats[0],
		},
		{
			Codec: "avc3.640028", Media: desc.Medias[4],
			Format: desc.Medias[4].Formats[0],
		},
		{
			Codec: "opus", Samplerate: 48000, Channels: 2,
			Media: desc.Medias[5], Format: desc.Medias[5].Formats[0],
		},
		{
			Codec: "flac", Samplerate: 44100, Channels: 2,
			InitData: base64.StdEncoding.EncodeToString(flacEnc), Media: desc.Medias[6], Format: desc.Medias[6].Formats[0],
		},
		{
			Codec: "mp4a.40.2", Samplerate: 44100, Channels: 2,
			InitData: base64.StdEncoding.EncodeToString(mpeg4Enc), Media: desc.Medias[7], Format: desc.Medias[7].Formats[0],
		},
		{
			Codec: "pcm-s16", Samplerate: 8000, Channels: 1,
			Media: desc.Medias[8], Format: desc.Medias[8].Formats[0],
		},
		{
			Codec: "pcm-s16", Samplerate: 44100, Channels: 2,
			Media: desc.Medias[9], Format: desc.Medias[9].Formats[0],
		},
	}, tracks)
}
