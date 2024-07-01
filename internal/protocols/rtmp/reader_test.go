package rtmp

import (
	"bytes"
	"testing"
	"time"

	"github.com/abema/go-mp4"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/amf0"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/bytecounter"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/h264conf"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/message"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestReadTracks(t *testing.T) {
	var spsp h265.SPS
	err := spsp.Unmarshal(test.FormatH265.SPS)
	require.NoError(t, err)

	hvcc := &mp4.HvcC{
		ConfigurationVersion:        1,
		GeneralProfileIdc:           spsp.ProfileTierLevel.GeneralProfileIdc,
		GeneralProfileCompatibility: spsp.ProfileTierLevel.GeneralProfileCompatibilityFlag,
		GeneralConstraintIndicator: [6]uint8{
			test.FormatH265.SPS[7], test.FormatH265.SPS[8], test.FormatH265.SPS[9],
			test.FormatH265.SPS[10], test.FormatH265.SPS[11], test.FormatH265.SPS[12],
		},
		GeneralLevelIdc: spsp.ProfileTierLevel.GeneralLevelIdc,
		// MinSpatialSegmentationIdc
		// ParallelismType
		ChromaFormatIdc:      uint8(spsp.ChromaFormatIdc),
		BitDepthLumaMinus8:   uint8(spsp.BitDepthLumaMinus8),
		BitDepthChromaMinus8: uint8(spsp.BitDepthChromaMinus8),
		// AvgFrameRate
		// ConstantFrameRate
		NumTemporalLayers: 1,
		// TemporalIdNested
		LengthSizeMinusOne: 3,
		NumOfNaluArrays:    3,
		NaluArrays: []mp4.HEVCNaluArray{
			{
				NaluType: byte(h265.NALUType_VPS_NUT),
				NumNalus: 1,
				Nalus: []mp4.HEVCNalu{{
					Length:  uint16(len(test.FormatH265.VPS)),
					NALUnit: test.FormatH265.VPS,
				}},
			},
			{
				NaluType: byte(h265.NALUType_SPS_NUT),
				NumNalus: 1,
				Nalus: []mp4.HEVCNalu{{
					Length:  uint16(len(test.FormatH265.SPS)),
					NALUnit: test.FormatH265.SPS,
				}},
			},
			{
				NaluType: byte(h265.NALUType_PPS_NUT),
				NumNalus: 1,
				Nalus: []mp4.HEVCNalu{{
					Length:  uint16(len(test.FormatH265.PPS)),
					NALUnit: test.FormatH265.PPS,
				}},
			},
		},
	}

	for _, ca := range []struct {
		name       string
		videoTrack format.Format
		audioTrack format.Format
		messages   []message.Message
	}{
		{
			"h264 + aac",
			&format.H264{
				PayloadTyp:        96,
				SPS:               test.FormatH264.SPS,
				PPS:               test.FormatH264.PPS,
				PacketizationMode: 1,
			},
			&format.MPEG4Audio{
				PayloadTyp: 96,
				Config: &mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{
								Key:   "videodatarate",
								Value: float64(0),
							},
							{
								Key:   "videocodecid",
								Value: float64(message.CodecH264),
							},
							{
								Key:   "audiodatarate",
								Value: float64(0),
							},
							{
								Key:   "audiocodecid",
								Value: float64(message.CodecMPEG4Audio),
							},
						},
					},
				},
				&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload: func() []byte {
						buf, _ := h264conf.Conf{
							SPS: test.FormatH264.SPS,
							PPS: test.FormatH264.PPS,
						}.Marshal()
						return buf
					}(),
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        true,
					AACType:         message.AudioAACTypeConfig,
					Payload: func() []byte {
						enc, err2 := mpeg4audio.Config{
							Type:         2,
							SampleRate:   44100,
							ChannelCount: 2,
						}.Marshal()
						require.NoError(t, err2)
						return enc
					}(),
				},
			},
		},
		{
			"h264",
			&format.H264{
				PayloadTyp:        96,
				SPS:               test.FormatH264.SPS,
				PPS:               test.FormatH264.PPS,
				PacketizationMode: 1,
			},
			nil,
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{
								Key:   "videodatarate",
								Value: float64(0),
							},
							{
								Key:   "videocodecid",
								Value: float64(message.CodecH264),
							},
							{
								Key:   "audiodatarate",
								Value: float64(0),
							},
							{
								Key:   "audiocodecid",
								Value: float64(0),
							},
						},
					},
				},
				&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload: func() []byte {
						buf, _ := h264conf.Conf{
							SPS: test.FormatH264.SPS,
							PPS: test.FormatH264.PPS,
						}.Marshal()
						return buf
					}(),
				},
			},
		},
		{
			"h264 + aac, issue mediamtx/386 (missing metadata)",
			&format.H264{
				PayloadTyp:        96,
				SPS:               test.FormatH264.SPS,
				PPS:               test.FormatH264.PPS,
				PacketizationMode: 1,
			},
			&format.MPEG4Audio{
				PayloadTyp: 96,
				Config: &mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			},
			[]message.Message{
				&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload: func() []byte {
						buf, _ := h264conf.Conf{
							SPS: test.FormatH264.SPS,
							PPS: test.FormatH264.PPS,
						}.Marshal()
						return buf
					}(),
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        true,
					AACType:         message.AudioAACTypeConfig,
					Payload: func() []byte {
						enc, err2 := mpeg4audio.Config{
							Type:         2,
							SampleRate:   44100,
							ChannelCount: 2,
						}.Marshal()
						require.NoError(t, err2)
						return enc
					}(),
				},
			},
		},
		{
			"h264 + aac, issue mediamtx/3301 (metadata without tracks)",
			&format.H264{
				PayloadTyp:        96,
				SPS:               test.FormatH264.SPS,
				PPS:               test.FormatH264.PPS,
				PacketizationMode: 1,
			},
			&format.MPEG4Audio{
				PayloadTyp: 96,
				Config: &mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{
								Key:   "metadatacreator",
								Value: "Agora.io SDK",
							},
							{
								Key:   "encoder",
								Value: "Agora.io Encoder",
							},
						},
					},
				},
				&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload: func() []byte {
						buf, _ := h264conf.Conf{
							SPS: test.FormatH264.SPS,
							PPS: test.FormatH264.PPS,
						}.Marshal()
						return buf
					}(),
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        true,
					AACType:         message.AudioAACTypeConfig,
					Payload: func() []byte {
						enc, err2 := mpeg4audio.Config{
							Type:         2,
							SampleRate:   44100,
							ChannelCount: 2,
						}.Marshal()
						require.NoError(t, err2)
						return enc
					}(),
				},
			},
		},
		{
			"aac, issue mediamtx/386 (missing metadata)",
			nil,
			&format.MPEG4Audio{
				PayloadTyp: 96,
				Config: &mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			},
			[]message.Message{
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        true,
					AACType:         message.AudioAACTypeConfig,
					Payload: func() []byte {
						enc, err2 := mpeg4audio.Config{
							Type:         2,
							SampleRate:   44100,
							ChannelCount: 2,
						}.Marshal()
						require.NoError(t, err2)
						return enc
					}(),
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        true,
					AACType:         message.AudioAACTypeConfig,
					Payload: func() []byte {
						enc, err2 := mpeg4audio.Config{
							Type:         2,
							SampleRate:   44100,
							ChannelCount: 2,
						}.Marshal()
						require.NoError(t, err2)
						return enc
					}(),
					DTS: 1 * time.Second,
				},
			},
		},
		{
			"aac, issue mediamtx/3414 (empty audio payload)",
			nil,
			&format.MPEG4Audio{
				PayloadTyp: 96,
				Config: &mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{
								Key:   "videodatarate",
								Value: float64(0),
							},
							{
								Key:   "videocodecid",
								Value: float64(0),
							},
							{
								Key:   "audiodatarate",
								Value: float64(0),
							},
							{
								Key:   "audiocodecid",
								Value: float64(message.CodecMPEG4Audio),
							},
						},
					},
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        true,
					AACType:         message.AudioAACTypeConfig,
					Payload:         nil,
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        true,
					AACType:         message.AudioAACTypeConfig,
					Payload: func() []byte {
						enc, err2 := mpeg4audio.Config{
							Type:         2,
							SampleRate:   44100,
							ChannelCount: 2,
						}.Marshal()
						require.NoError(t, err2)
						return enc
					}(),
				},
			},
		},
		{
			"h265 + aac, obs studio pre 29.1 h265",
			&format.H265{
				PayloadTyp: 96,
				VPS:        test.FormatH265.VPS,
				SPS:        test.FormatH265.SPS,
				PPS:        test.FormatH265.PPS,
			},
			&format.MPEG4Audio{
				PayloadTyp: 96,
				Config: &mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{
								Key:   "videodatarate",
								Value: float64(0),
							},
							{
								Key:   "videocodecid",
								Value: float64(message.CodecH264),
							},
							{
								Key:   "audiodatarate",
								Value: float64(0),
							},
							{
								Key:   "audiocodecid",
								Value: float64(message.CodecMPEG4Audio),
							},
						},
					},
				},
				&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeAU,
					Payload: func() []byte {
						avcc, err2 := h264.AVCCMarshal([][]byte{
							test.FormatH265.VPS,
							test.FormatH265.SPS,
							test.FormatH265.PPS,
						})
						require.NoError(t, err2)
						return avcc
					}(),
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        true,
					AACType:         message.AudioAACTypeConfig,
					Payload: func() []byte {
						enc, err2 := mpeg4audio.Config{
							Type:         2,
							SampleRate:   44100,
							ChannelCount: 2,
						}.Marshal()
						require.NoError(t, err2)
						return enc
					}(),
				},
			},
		},
		{
			"h265, issue mediamtx/2232 (xsplit broadcaster)",
			&format.H265{
				PayloadTyp: 96,
				VPS:        test.FormatH265.VPS,
				SPS:        test.FormatH265.SPS,
				PPS:        test.FormatH265.PPS,
			},
			nil,
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{
								Key:   "videodatarate",
								Value: float64(0),
							},
							{
								Key:   "videocodecid",
								Value: "hvc1",
							},
							{
								Key:   "audiodatarate",
								Value: float64(0),
							},
							{
								Key:   "audiocodecid",
								Value: float64(0),
							},
						},
					},
				},
				&message.ExtendedSequenceStart{
					ChunkStreamID:   4,
					MessageStreamID: 0x1000000,
					FourCC:          message.FourCCHEVC,
					Config: func() []byte {
						var buf bytes.Buffer
						_, err = mp4.Marshal(&buf, hvcc, mp4.Context{})
						require.NoError(t, err)
						return buf.Bytes()
					}(),
				},
			},
		},
		{
			"h265, obs 30.0",
			&format.H265{
				PayloadTyp: 96,
				VPS:        test.FormatH265.VPS,
				SPS:        test.FormatH265.SPS,
				PPS:        test.FormatH265.PPS,
			},
			nil,
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{
								Key:   "videodatarate",
								Value: float64(0),
							},
							{
								Key:   "videocodecid",
								Value: float64(message.FourCCHEVC),
							},
							{
								Key:   "audiodatarate",
								Value: float64(0),
							},
							{
								Key:   "audiocodecid",
								Value: float64(0),
							},
						},
					},
				},
				&message.ExtendedSequenceStart{
					ChunkStreamID:   4,
					MessageStreamID: 0x1000000,
					FourCC:          message.FourCCHEVC,
					Config: func() []byte {
						var buf bytes.Buffer
						_, err = mp4.Marshal(&buf, hvcc, mp4.Context{})
						require.NoError(t, err)
						return buf.Bytes()
					}(),
				},
			},
		},
		{
			"av1, ffmpeg",
			&format.AV1{
				PayloadTyp: 96,
			},
			nil,
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{
								Key:   "duration",
								Value: float64(0),
							},
							{
								Key:   "width",
								Value: float64(1920),
							},
							{
								Key:   "height",
								Value: float64(1080),
							},
							{
								Key:   "videodatarate",
								Value: float64(0),
							},
							{
								Key:   "framerate",
								Value: float64(30),
							},
							{
								Key:   "videocodecid",
								Value: float64(message.FourCCAV1),
							},
							{
								Key:   "encoder",
								Value: "Lavf60.10.101",
							},
							{
								Key:   "filesize",
								Value: float64(0),
							},
						},
					},
				},
				&message.ExtendedSequenceStart{
					ChunkStreamID:   6,
					MessageStreamID: 0x1000000,
					FourCC:          message.FourCCAV1,
					Config: []byte{
						0x81, 0x08, 0x0c, 0x00, 0x0a, 0x0b, 0x00, 0x00,
						0x00, 0x42, 0xab, 0xbf, 0xc3, 0x70, 0x0b, 0xe0,
						0x01,
					},
				},
			},
		},
		{
			"h264 + aac, issue mediamtx/2289 (missing videocodecid)",
			&format.H264{
				PayloadTyp: 96,
				SPS: []byte{
					0x67, 0x64, 0x00, 0x1f, 0xac, 0x2c, 0x6a, 0x81,
					0x40, 0x16, 0xe9, 0xb8, 0x28, 0x08, 0x2a, 0x00,
					0x00, 0x03, 0x00, 0x02, 0x00, 0x00, 0x03, 0x00,
					0xc9, 0x08,
				},
				PPS:               []byte{0x68, 0xee, 0x31, 0xb2, 0x1b},
				PacketizationMode: 1,
			},
			&format.MPEG4Audio{
				PayloadTyp: 96,
				Config: &mpeg4audio.Config{
					Type:         2,
					SampleRate:   48000,
					ChannelCount: 1,
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{
								Key:   "width",
								Value: float64(1280),
							},
							{
								Key:   "height",
								Value: float64(720),
							},
							{
								Key:   "framerate",
								Value: float64(30),
							},
							{
								Key:   "audiocodecid",
								Value: float64(10),
							},
						},
					},
				},
				&message.Video{
					ChunkStreamID:   0x15,
					MessageStreamID: 0x1000000,
					Codec:           0x7,
					IsKeyFrame:      true,
					Payload: []uint8{
						0x01, 0x64, 0x00, 0x1f, 0xff, 0xe1, 0x00, 0x1a,
						0x67, 0x64, 0x00, 0x1f, 0xac, 0x2c, 0x6a, 0x81,
						0x40, 0x16, 0xe9, 0xb8, 0x28, 0x08, 0x2a, 0x00,
						0x00, 0x03, 0x00, 0x02, 0x00, 0x00, 0x03, 0x00,
						0xc9, 0x08, 0x01, 0x00, 0x05, 0x68, 0xee, 0x31,
						0xb2, 0x1b,
					},
				},
				&message.Audio{
					ChunkStreamID:   0x14,
					MessageStreamID: 0x1000000,
					Codec:           0xa,
					Rate:            0x3,
					Depth:           0x1,
					IsStereo:        true,
					Payload:         []uint8{0x11, 0x88},
				},
			},
		},
		{
			"h264, issue mediamtx/2352",
			&format.H264{
				PayloadTyp:        96,
				SPS:               test.FormatH264.SPS,
				PPS:               test.FormatH264.PPS,
				PacketizationMode: 1,
			},
			nil,
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   8,
					MessageStreamID: 0x1000000,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{
								Key:   "audiodatarate",
								Value: float64(128),
							},
							{
								Key:   "framerate",
								Value: float64(30),
							},
							{
								Key:   "videocodecid",
								Value: float64(7),
							},
							{
								Key:   "videodatarate",
								Value: float64(2500),
							},
							{
								Key:   "audiocodecid",
								Value: float64(10),
							},
							{
								Key:   "height",
								Value: float64(720),
							},
							{
								Key:   "width",
								Value: float64(1280),
							},
						},
					},
				},
				&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload: func() []byte {
						buf, _ := h264conf.Conf{
							SPS: test.FormatH264.SPS,
							PPS: test.FormatH264.PPS,
						}.Marshal()
						return buf
					}(),
				},
				&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           0x7,
					IsKeyFrame:      true,
					Payload: []uint8{
						5,
					},
				},
				&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           0x7,
					IsKeyFrame:      true,
					DTS:             2 * time.Second,
					Payload: []uint8{
						5,
					},
				},
			},
		},
		{
			"mpeg-1 audio",
			nil,
			&format.MPEG1Audio{},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{Key: "duration", Value: float64(0)},
							{Key: "audiocodecid", Value: float64(2)},
							{Key: "encoder", Value: "Lavf58.45.100"},
							{Key: "filesize", Value: float64(0)},
						},
					},
				},
			},
		},
		{
			"pcma",
			nil,
			&format.G711{
				PayloadTyp:   8,
				MULaw:        false,
				SampleRate:   8000,
				ChannelCount: 1,
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{Key: "duration", Value: float64(0)},
							{Key: "audiocodecid", Value: float64(7)},
							{Key: "encoder", Value: "Lavf58.45.100"},
							{Key: "filesize", Value: float64(0)},
						},
					},
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecPCMA,
					Rate:            message.Rate5512,
					Depth:           message.Depth16,
					IsStereo:        false,
					Payload:         []byte{1, 2, 3, 4},
				},
			},
		},
		{
			"pcmu",
			nil,
			&format.G711{
				PayloadTyp:   0,
				MULaw:        true,
				SampleRate:   8000,
				ChannelCount: 1,
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{Key: "duration", Value: float64(0)},
							{Key: "audiocodecid", Value: float64(8)},
							{Key: "encoder", Value: "Lavf58.45.100"},
							{Key: "filesize", Value: float64(0)},
						},
					},
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecPCMU,
					Rate:            message.Rate5512,
					Depth:           message.Depth16,
					IsStereo:        false,
					Payload:         []byte{1, 2, 3, 4},
				},
			},
		},
		{
			"lpcm gstreamer",
			nil,
			&format.LPCM{
				PayloadTyp:   96,
				BitDepth:     16,
				SampleRate:   44100,
				ChannelCount: 2,
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{Key: "duration", Value: float64(0)},
							{Key: "audiocodecid", Value: float64(3)},
							{Key: "filesize", Value: float64(0)},
						},
					},
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecLPCM,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        true,
					Payload:         []byte{1, 2, 3, 4},
				},
			},
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			var buf bytes.Buffer
			bc := bytecounter.NewReadWriter(&buf)
			mrw := message.NewReadWriter(bc, bc, true)

			for _, msg := range ca.messages {
				err := mrw.Write(msg)
				require.NoError(t, err)
			}

			c := newNoHandshakeConn(&buf)

			r, err := NewReader(c)
			require.NoError(t, err)
			videoTrack, audioTrack := r.Tracks()
			require.Equal(t, ca.videoTrack, videoTrack)
			require.Equal(t, ca.audioTrack, audioTrack)
		})
	}
}
