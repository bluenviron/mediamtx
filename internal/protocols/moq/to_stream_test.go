package moq

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/catalog"
	"github.com/stretchr/testify/require"
)

func TestToStream(t *testing.T) {
	cat := &catalog.Catalog{
		Tracks: []catalog.Track{
			{Codec: "av01.0.04M.08"},
			{Codec: "vp09.00.10.08"},
			{Codec: "vp8"},
			{Codec: "hev1.1.6.L93.B0"},
			{Codec: "avc3.640028"},
			{Codec: "opus"},
			{Codec: "mp4a.40.2", Samplerate: 44100, Channels: 2},
		},
	}

	medias, _, err := ToStream(cat, nil)
	require.NoError(t, err)

	require.Equal(t, []*description.Media{
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.AV1{PayloadTyp: 96}},
		},
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.VP9{PayloadTyp: 96}},
		},
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.VP8{PayloadTyp: 96}},
		},
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.H265{PayloadTyp: 96}},
		},
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.H264{PayloadTyp: 96, PacketizationMode: 1}},
		},
		{
			Type:    description.MediaTypeAudio,
			Formats: []format.Format{&format.Opus{PayloadTyp: 96, ChannelCount: 2}},
		},
		{
			Type: description.MediaTypeAudio,
			Formats: []format.Format{&format.MPEG4Audio{
				PayloadTyp: 96,
				Config: &mpeg4audio.AudioSpecificConfig{
					Type:          mpeg4audio.ObjectTypeAACLC,
					SampleRate:    44100,
					ChannelConfig: 2,
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			}},
		},
	}, medias)
}
