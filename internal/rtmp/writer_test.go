package rtmp

import (
	"bytes"
	"testing"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/notedit/rtmp/format/flv/flvio"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/rtmp/bytecounter"
	"github.com/bluenviron/mediamtx/internal/rtmp/message"
)

func TestWriteTracks(t *testing.T) {
	videoTrack := &formats.H264{
		PayloadTyp: 96,
		SPS: []byte{
			0x67, 0x64, 0x00, 0x0c, 0xac, 0x3b, 0x50, 0xb0,
			0x4b, 0x42, 0x00, 0x00, 0x03, 0x00, 0x02, 0x00,
			0x00, 0x03, 0x00, 0x3d, 0x08,
		},
		PPS: []byte{
			0x68, 0xee, 0x3c, 0x80,
		},
		PacketizationMode: 1,
	}

	audioTrack := &formats.MPEG4Audio{
		PayloadTyp: 96,
		Config: &mpeg4audio.Config{
			Type:         2,
			SampleRate:   44100,
			ChannelCount: 2,
		},
		SizeLength:       13,
		IndexLength:      3,
		IndexDeltaLength: 3,
	}

	var buf bytes.Buffer
	c := newNoHandshakeConn(&buf)

	_, err := NewWriter(c, videoTrack, audioTrack)
	require.NoError(t, err)

	bc := bytecounter.NewReadWriter(&buf)
	mrw := message.NewReadWriter(bc, bc, true)

	msg, err := mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.DataAMF0{
		ChunkStreamID:   4,
		MessageStreamID: 0x1000000,
		Payload: []interface{}{
			"@setDataFrame",
			"onMetaData",
			flvio.AMFMap{
				{K: "videodatarate", V: float64(0)},
				{K: "videocodecid", V: float64(7)},
				{K: "audiodatarate", V: float64(0)},
				{K: "audiocodecid", V: float64(10)},
			},
		},
	}, msg)

	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.Video{
		ChunkStreamID:   message.VideoChunkStreamID,
		MessageStreamID: 0x1000000,
		Codec:           message.CodecH264,
		IsKeyFrame:      true,
		Type:            message.VideoTypeConfig,
		Payload: []byte{
			0x1, 0x64, 0x0,
			0xc, 0xff, 0xe1, 0x0, 0x15, 0x67, 0x64, 0x0,
			0xc, 0xac, 0x3b, 0x50, 0xb0, 0x4b, 0x42, 0x0,
			0x0, 0x3, 0x0, 0x2, 0x0, 0x0, 0x3, 0x0,
			0x3d, 0x8, 0x1, 0x0, 0x4, 0x68, 0xee, 0x3c,
			0x80,
		},
	}, msg)

	msg, err = mrw.Read()
	require.NoError(t, err)
	require.Equal(t, &message.Audio{
		ChunkStreamID:   message.AudioChunkStreamID,
		MessageStreamID: 0x1000000,
		Codec:           message.CodecMPEG4Audio,
		Rate:            flvio.SOUND_44Khz,
		Depth:           flvio.SOUND_16BIT,
		Channels:        flvio.SOUND_STEREO,
		AACType:         message.AudioAACTypeConfig,
		Payload:         []byte{0x12, 0x10},
	}, msg)
}
