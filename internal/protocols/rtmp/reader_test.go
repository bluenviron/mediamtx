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
	"github.com/notedit/rtmp/format/flv/flvio"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/bytecounter"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/h264conf"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/message"
)

func TestReadTracks(t *testing.T) {
	h264SPS := []byte{
		0x67, 0x64, 0x00, 0x0c, 0xac, 0x3b, 0x50, 0xb0,
		0x4b, 0x42, 0x00, 0x00, 0x03, 0x00, 0x02, 0x00,
		0x00, 0x03, 0x00, 0x3d, 0x08,
	}

	h264PPS := []byte{
		0x68, 0xee, 0x3c, 0x80,
	}

	h265VPS := []byte{
		0x40, 0x01, 0x0c, 0x01, 0xff, 0xff, 0x01, 0x40,
		0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x00,
		0x03, 0x00, 0x00, 0x03, 0x00, 0x7b, 0xac, 0x09,
	}

	h265SPS := []byte{
		0x42, 0x01, 0x01, 0x01, 0x40, 0x00, 0x00, 0x03,
		0x00, 0x00, 0x03, 0x00, 0x00, 0x03, 0x00, 0x00,
		0x03, 0x00, 0x7b, 0xa0, 0x03, 0xc0, 0x80, 0x11,
		0x07, 0xcb, 0x96, 0xb4, 0xa4, 0x25, 0x92, 0xe3,
		0x01, 0x6a, 0x02, 0x02, 0x02, 0x08, 0x00, 0x00,
		0x03, 0x00, 0x08, 0x00, 0x00, 0x03, 0x01, 0xe3,
		0x00, 0x2e, 0xf2, 0x88, 0x00, 0x09, 0x89, 0x60,
		0x00, 0x04, 0xc4, 0xb4, 0x20,
	}

	h265PPS := []byte{
		0x44, 0x01, 0xc0, 0xf7, 0xc0, 0xcc, 0x90,
	}

	var spsp h265.SPS
	err := spsp.Unmarshal(h265SPS)
	require.NoError(t, err)

	hvcc := &mp4.HvcC{
		ConfigurationVersion:        1,
		GeneralProfileIdc:           spsp.ProfileTierLevel.GeneralProfileIdc,
		GeneralProfileCompatibility: spsp.ProfileTierLevel.GeneralProfileCompatibilityFlag,
		GeneralConstraintIndicator: [6]uint8{
			h265SPS[7], h265SPS[8], h265SPS[9],
			h265SPS[10], h265SPS[11], h265SPS[12],
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
					Length:  uint16(len(h265VPS)),
					NALUnit: h265VPS,
				}},
			},
			{
				NaluType: byte(h265.NALUType_SPS_NUT),
				NumNalus: 1,
				Nalus: []mp4.HEVCNalu{{
					Length:  uint16(len(h265SPS)),
					NALUnit: h265SPS,
				}},
			},
			{
				NaluType: byte(h265.NALUType_PPS_NUT),
				NumNalus: 1,
				Nalus: []mp4.HEVCNalu{{
					Length:  uint16(len(h265PPS)),
					NALUnit: h265PPS,
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
				SPS:               h264SPS,
				PPS:               h264PPS,
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
						flvio.AMFMap{
							{
								K: "videodatarate",
								V: float64(0),
							},
							{
								K: "videocodecid",
								V: float64(message.CodecH264),
							},
							{
								K: "audiodatarate",
								V: float64(0),
							},
							{
								K: "audiocodecid",
								V: float64(message.CodecMPEG4Audio),
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
							SPS: h264SPS,
							PPS: h264PPS,
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
						enc, err := mpeg4audio.Config{
							Type:         2,
							SampleRate:   44100,
							ChannelCount: 2,
						}.Marshal()
						require.NoError(t, err)
						return enc
					}(),
				},
			},
		},
		{
			"h264",
			&format.H264{
				PayloadTyp:        96,
				SPS:               h264SPS,
				PPS:               h264PPS,
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
						flvio.AMFMap{
							{
								K: "videodatarate",
								V: float64(0),
							},
							{
								K: "videocodecid",
								V: float64(message.CodecH264),
							},
							{
								K: "audiodatarate",
								V: float64(0),
							},
							{
								K: "audiocodecid",
								V: float64(0),
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
							SPS: h264SPS,
							PPS: h264PPS,
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
				SPS:               h264SPS,
				PPS:               h264PPS,
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
							SPS: h264SPS,
							PPS: h264PPS,
						}.Marshal()
						return buf
					}(),
				},
				&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload: func() []byte {
						buf, _ := h264conf.Conf{
							SPS: h264SPS,
							PPS: h264PPS,
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
						enc, err := mpeg4audio.Config{
							Type:         2,
							SampleRate:   44100,
							ChannelCount: 2,
						}.Marshal()
						require.NoError(t, err)
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
						enc, err := mpeg4audio.Config{
							Type:         2,
							SampleRate:   44100,
							ChannelCount: 2,
						}.Marshal()
						require.NoError(t, err)
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
						enc, err := mpeg4audio.Config{
							Type:         2,
							SampleRate:   44100,
							ChannelCount: 2,
						}.Marshal()
						require.NoError(t, err)
						return enc
					}(),
					DTS: 1 * time.Second,
				},
			},
		},
		{
			"h265 + aac, obs studio pre 29.1 h265",
			&format.H265{
				PayloadTyp: 96,
				VPS:        h265VPS,
				SPS:        h265SPS,
				PPS:        h265PPS,
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
						flvio.AMFMap{
							{
								K: "videodatarate",
								V: float64(0),
							},
							{
								K: "videocodecid",
								V: float64(message.CodecH264),
							},
							{
								K: "audiodatarate",
								V: float64(0),
							},
							{
								K: "audiocodecid",
								V: float64(message.CodecMPEG4Audio),
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
						avcc, err := h264.AVCCMarshal([][]byte{
							h265VPS,
							h265SPS,
							h265PPS,
						})
						require.NoError(t, err)
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
						enc, err := mpeg4audio.Config{
							Type:         2,
							SampleRate:   44100,
							ChannelCount: 2,
						}.Marshal()
						require.NoError(t, err)
						return enc
					}(),
				},
			},
		},
		{
			"h265, issue mediamtx/2232 (xsplit broadcaster)",
			&format.H265{
				PayloadTyp: 96,
				VPS:        h265VPS,
				SPS:        h265SPS,
				PPS:        h265PPS,
			},
			nil,
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						flvio.AMFMap{
							{
								K: "videodatarate",
								V: float64(0),
							},
							{
								K: "videocodecid",
								V: "hvc1",
							},
							{
								K: "audiodatarate",
								V: float64(0),
							},
							{
								K: "audiocodecid",
								V: float64(0),
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
				VPS:        h265VPS,
				SPS:        h265SPS,
				PPS:        h265PPS,
			},
			nil,
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						flvio.AMFMap{
							{
								K: "videodatarate",
								V: float64(0),
							},
							{
								K: "videocodecid",
								V: float64(message.FourCCHEVC),
							},
							{
								K: "audiodatarate",
								V: float64(0),
							},
							{
								K: "audiocodecid",
								V: float64(0),
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
						flvio.AMFMap{
							{
								K: "duration",
								V: float64(0),
							},
							{
								K: "width",
								V: float64(1920),
							},
							{
								K: "height",
								V: float64(1080),
							},
							{
								K: "videodatarate",
								V: float64(0),
							},
							{
								K: "framerate",
								V: float64(30),
							},
							{
								K: "videocodecid",
								V: float64(message.FourCCAV1),
							},
							{
								K: "encoder",
								V: "Lavf60.10.101",
							},
							{
								K: "filesize",
								V: float64(0),
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
						flvio.AMFMap{
							{
								K: "width",
								V: float64(1280),
							},
							{
								K: "height",
								V: float64(720),
							},
							{
								K: "framerate",
								V: float64(30),
							},
							{
								K: "audiocodecid",
								V: float64(10),
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
				SPS:               h264SPS,
				PPS:               h264PPS,
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
						flvio.AMFMap{
							{
								K: "audiodatarate",
								V: float64(128),
							},
							{
								K: "framerate",
								V: float64(30),
							},
							{
								K: "videocodecid",
								V: float64(7),
							},
							{
								K: "videodatarate",
								V: float64(2500),
							},
							{
								K: "audiocodecid",
								V: float64(10),
							},
							{
								K: "height",
								V: float64(720),
							},
							{
								K: "width",
								V: float64(1280),
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
							SPS: h264SPS,
							PPS: h264PPS,
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
						flvio.AMFMap{
							{K: "duration", V: 0},
							{K: "audiocodecid", V: 2},
							{K: "encoder", V: "Lavf58.45.100"},
							{K: "filesize", V: 0},
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
						flvio.AMFMap{
							{K: "duration", V: 0},
							{K: "audiocodecid", V: 7},
							{K: "encoder", V: "Lavf58.45.100"},
							{K: "filesize", V: 0},
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
						flvio.AMFMap{
							{K: "duration", V: 0},
							{K: "audiocodecid", V: 8},
							{K: "encoder", V: "Lavf58.45.100"},
							{K: "filesize", V: 0},
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
						flvio.AMFMap{
							{K: "duration", V: 0},
							{K: "audiocodecid", V: 3},
							{K: "filesize", V: 0},
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
