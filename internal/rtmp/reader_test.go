package rtmp

import (
	"bytes"
	"testing"
	"time"

	"github.com/abema/go-mp4"
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/notedit/rtmp/format/flv/flvio"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/rtmp/bytecounter"
	"github.com/bluenviron/mediamtx/internal/rtmp/h264conf"
	"github.com/bluenviron/mediamtx/internal/rtmp/message"
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
		videoTrack formats.Format
		audioTrack formats.Format
	}{
		{
			"video+audio",
			&formats.H264{
				PayloadTyp:        96,
				SPS:               h264SPS,
				PPS:               h264PPS,
				PacketizationMode: 1,
			},
			&formats.MPEG4Audio{
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
		},
		{
			"video",
			&formats.H264{
				PayloadTyp:        96,
				SPS:               h264SPS,
				PPS:               h264PPS,
				PacketizationMode: 1,
			},
			nil,
		},
		{
			"metadata without codec id, video+audio",
			&formats.H264{
				PayloadTyp:        96,
				SPS:               h264SPS,
				PPS:               h264PPS,
				PacketizationMode: 1,
			},
			&formats.MPEG4Audio{
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
		},
		{
			"metadata without codec id, video only",
			&formats.H264{
				PayloadTyp:        96,
				SPS:               h264SPS,
				PPS:               h264PPS,
				PacketizationMode: 1,
			},
			nil,
		},
		{
			"missing metadata, video+audio",
			&formats.H264{
				PayloadTyp:        96,
				SPS:               h264SPS,
				PPS:               h264PPS,
				PacketizationMode: 1,
			},
			&formats.MPEG4Audio{
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
		},
		{
			"missing metadata, audio",
			nil,
			&formats.MPEG4Audio{
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
		},
		{
			"obs studio pre 29.1 h265",
			&formats.H265{
				PayloadTyp: 96,
				VPS:        h265VPS,
				SPS:        h265SPS,
				PPS:        h265PPS,
			},
			&formats.MPEG4Audio{
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
		},
		{
			"xplit broadcaster",
			&formats.H265{
				PayloadTyp: 96,
				VPS:        h265VPS,
				SPS:        h265SPS,
				PPS:        h265PPS,
			},
			nil,
		},
		{
			"obs 30",
			&formats.H265{
				PayloadTyp: 96,
				VPS:        h265VPS,
				SPS:        h265SPS,
				PPS:        h265PPS,
			},
			nil,
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			var buf bytes.Buffer
			bc := bytecounter.NewReadWriter(&buf)
			mrw := message.NewReadWriter(bc, bc, true)

			switch ca.name {
			case "video+audio":
				err := mrw.Write(&message.DataAMF0{
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
				})
				require.NoError(t, err)

				buf, _ := h264conf.Conf{
					SPS: h264SPS,
					PPS: h264PPS,
				}.Marshal()

				err = mrw.Write(&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload:         buf,
				})
				require.NoError(t, err)

				enc, err := mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				}.Marshal()
				require.NoError(t, err)

				err = mrw.Write(&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            flvio.SOUND_44Khz,
					Depth:           flvio.SOUND_16BIT,
					Channels:        flvio.SOUND_STEREO,
					AACType:         message.AudioAACTypeConfig,
					Payload:         enc,
				})
				require.NoError(t, err)

			case "video":
				err := mrw.Write(&message.DataAMF0{
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
				})
				require.NoError(t, err)

				buf, _ := h264conf.Conf{
					SPS: h264SPS,
					PPS: h264PPS,
				}.Marshal()

				err = mrw.Write(&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload:         buf,
				})
				require.NoError(t, err)

			case "metadata without codec id, video+audio":
				err := mrw.Write(&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						flvio.AMFMap{
							{
								K: "width",
								V: float64(2688),
							},
							{
								K: "height",
								V: float64(1520),
							},
							{
								K: "framerate",
								V: float64(0o25),
							},
						},
					},
				})
				require.NoError(t, err)

				buf, _ := h264conf.Conf{
					SPS: h264SPS,
					PPS: h264PPS,
				}.Marshal()

				err = mrw.Write(&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload:         buf,
				})
				require.NoError(t, err)

				enc, err := mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				}.Marshal()
				require.NoError(t, err)

				err = mrw.Write(&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            flvio.SOUND_44Khz,
					Depth:           flvio.SOUND_16BIT,
					Channels:        flvio.SOUND_STEREO,
					AACType:         message.AudioAACTypeConfig,
					Payload:         enc,
				})
				require.NoError(t, err)

			case "metadata without codec id, video only":
				err := mrw.Write(&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						flvio.AMFMap{
							{
								K: "width",
								V: float64(2688),
							},
							{
								K: "height",
								V: float64(1520),
							},
							{
								K: "framerate",
								V: float64(0o25),
							},
						},
					},
				})
				require.NoError(t, err)

				buf, _ := h264conf.Conf{
					SPS: h264SPS,
					PPS: h264PPS,
				}.Marshal()

				err = mrw.Write(&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload:         buf,
				})
				require.NoError(t, err)

				err = mrw.Write(&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeAU,
					Payload:         []byte{0x01, 0x02, 0x03, 0x04},
					DTS:             1 * time.Second,
				})
				require.NoError(t, err)

			case "missing metadata, video+audio":
				buf, _ := h264conf.Conf{
					SPS: h264SPS,
					PPS: h264PPS,
				}.Marshal()

				err := mrw.Write(&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload:         buf,
				})
				require.NoError(t, err)

				enc, err := mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				}.Marshal()
				require.NoError(t, err)

				err = mrw.Write(&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            flvio.SOUND_44Khz,
					Depth:           flvio.SOUND_16BIT,
					Channels:        flvio.SOUND_STEREO,
					AACType:         message.AudioAACTypeConfig,
					Payload:         enc,
				})
				require.NoError(t, err)

			case "missing metadata, audio":
				enc, err := mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				}.Marshal()
				require.NoError(t, err)

				err = mrw.Write(&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            flvio.SOUND_44Khz,
					Depth:           flvio.SOUND_16BIT,
					Channels:        flvio.SOUND_STEREO,
					AACType:         message.AudioAACTypeConfig,
					Payload:         enc,
				})
				require.NoError(t, err)

				err = mrw.Write(&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            flvio.SOUND_44Khz,
					Depth:           flvio.SOUND_16BIT,
					Channels:        flvio.SOUND_STEREO,
					AACType:         message.AudioAACTypeConfig,
					Payload:         enc,
					DTS:             1 * time.Second,
				})
				require.NoError(t, err)

			case "obs studio pre 29.1 h265":
				err := mrw.Write(&message.DataAMF0{
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
				})
				require.NoError(t, err)

				avcc, err := h264.AVCCMarshal([][]byte{
					h265VPS,
					h265SPS,
					h265PPS,
				})
				require.NoError(t, err)

				err = mrw.Write(&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeAU,
					Payload:         avcc,
				})
				require.NoError(t, err)

				enc, err := mpeg4audio.Config{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				}.Marshal()
				require.NoError(t, err)

				err = mrw.Write(&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            flvio.SOUND_44Khz,
					Depth:           flvio.SOUND_16BIT,
					Channels:        flvio.SOUND_STEREO,
					AACType:         message.AudioAACTypeConfig,
					Payload:         enc,
				})
				require.NoError(t, err)

			case "xplit broadcaster":
				err := mrw.Write(&message.DataAMF0{
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
				})
				require.NoError(t, err)

				var buf bytes.Buffer
				_, err = mp4.Marshal(&buf, hvcc, mp4.Context{})
				require.NoError(t, err)

				err = mrw.Write(&message.ExtendedSequenceStart{
					ChunkStreamID:   4,
					MessageStreamID: 0x1000000,
					FourCC:          message.FourCCHEVC,
					Config:          buf.Bytes(),
				})
				require.NoError(t, err)

			case "obs 30":
				err := mrw.Write(&message.DataAMF0{
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
				})
				require.NoError(t, err)

				var buf bytes.Buffer
				_, err = mp4.Marshal(&buf, hvcc, mp4.Context{})
				require.NoError(t, err)

				err = mrw.Write(&message.ExtendedSequenceStart{
					ChunkStreamID:   4,
					MessageStreamID: 0x1000000,
					FourCC:          message.FourCCHEVC,
					Config:          buf.Bytes(),
				})
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
