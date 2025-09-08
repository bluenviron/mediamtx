package rtmp

import (
	"fmt"
	"strconv"
	"time"

	"github.com/abema/go-mp4"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg1audio"

	"github.com/bluenviron/mediamtx/internal/formatprocessor"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/amf0"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/h264conf"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/message"
)

func codecID(track format.Format) float64 {
	switch track := track.(type) {
	// video

	case *format.AV1:
		return float64(message.FourCCAV1)

	case *format.VP9:
		return float64(message.FourCCVP9)

	case *format.H265:
		return float64(message.FourCCHEVC)

	case *format.H264:
		return message.CodecH264

		// audio

	case *format.Opus:
		return float64(message.FourCCOpus)

	case *format.MPEG4Audio, *format.MPEG4AudioLATM:
		return message.CodecMPEG4Audio

	case *format.MPEG1Audio:
		return message.CodecMPEG1Audio

	case *format.AC3:
		return float64(message.FourCCAC3)

	case *format.G711:
		if track.MULaw {
			return message.CodecPCMU
		}
		return message.CodecPCMA

	case *format.LPCM:
		return message.CodecLPCM
	}

	return 0
}

func generateHvcC(vps, sps, pps []byte) *mp4.HvcC {
	var psps h265.SPS
	err := psps.Unmarshal(sps)
	if err != nil {
		panic(err)
	}

	return &mp4.HvcC{
		ConfigurationVersion:        1,
		GeneralProfileIdc:           psps.ProfileTierLevel.GeneralProfileIdc,
		GeneralProfileCompatibility: psps.ProfileTierLevel.GeneralProfileCompatibilityFlag,
		GeneralConstraintIndicator: [6]uint8{
			sps[7], sps[8], sps[9],
			sps[10], sps[11], sps[12],
		},
		GeneralLevelIdc: psps.ProfileTierLevel.GeneralLevelIdc,
		// MinSpatialSegmentationIdc
		// ParallelismType
		ChromaFormatIdc:      uint8(psps.ChromaFormatIdc),
		BitDepthLumaMinus8:   uint8(psps.BitDepthLumaMinus8),
		BitDepthChromaMinus8: uint8(psps.BitDepthChromaMinus8),
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
					Length:  uint16(len(vps)),
					NALUnit: vps,
				}},
			},
			{
				NaluType: byte(h265.NALUType_SPS_NUT),
				NumNalus: 1,
				Nalus: []mp4.HEVCNalu{{
					Length:  uint16(len(sps)),
					NALUnit: sps,
				}},
			},
			{
				NaluType: byte(h265.NALUType_PPS_NUT),
				NumNalus: 1,
				Nalus: []mp4.HEVCNalu{{
					Length:  uint16(len(pps)),
					NALUnit: pps,
				}},
			},
		},
	}
}

func generateAvcC(sps, pps []byte) *mp4.AVCDecoderConfiguration {
	var psps h264.SPS
	err := psps.Unmarshal(sps)
	if err != nil {
		panic(err)
	}

	return &mp4.AVCDecoderConfiguration{ // <avcc/>
		AnyTypeBox: mp4.AnyTypeBox{
			Type: mp4.BoxTypeAvcC(),
		},
		ConfigurationVersion:       1,
		Profile:                    psps.ProfileIdc,
		ProfileCompatibility:       sps[2],
		Level:                      psps.LevelIdc,
		LengthSizeMinusOne:         3,
		NumOfSequenceParameterSets: 1,
		SequenceParameterSets: []mp4.AVCParameterSet{
			{
				Length:  uint16(len(sps)),
				NALUnit: sps,
			},
		},
		NumOfPictureParameterSets: 1,
		PictureParameterSets: []mp4.AVCParameterSet{
			{
				Length:  uint16(len(pps)),
				NALUnit: pps,
			},
		},
	}
}

func audioRateRTMPToInt(v uint8) int {
	switch v {
	case message.Rate5512:
		return 5512
	case message.Rate11025:
		return 11025
	case message.Rate22050:
		return 22050
	default:
		return 44100
	}
}

func audioRateIntToRTMP(v int) (uint8, bool) {
	switch v {
	case 5512:
		return message.Rate5512, true
	case 11025:
		return message.Rate11025, true
	case 22050:
		return message.Rate22050, true
	case 44100:
		return message.Rate44100, true
	}

	return 0, false
}

func mpeg1AudioChannels(m mpeg1audio.ChannelMode) bool {
	return m != mpeg1audio.ChannelModeMono
}

// Writer provides functions to write outgoing data.
type Writer struct {
	Conn   Conn
	Tracks []format.Format

	videoTrackToID map[format.Format]uint8
	audioTrackToID map[format.Format]uint8
}

// Initialize initializes Writer.
func (w *Writer) Initialize() error {
	err := w.writeTracks()
	if err != nil {
		return err
	}

	return nil
}

func (w *Writer) writeTracks() error {
	w.videoTrackToID = make(map[format.Format]uint8)
	w.audioTrackToID = make(map[format.Format]uint8)

	var videoTracks []format.Format
	var audioTracks []format.Format

	for _, track := range w.Tracks {
		switch track.(type) {
		case *format.AV1, *format.VP9, *format.H265, *format.H264:
			w.videoTrackToID[track] = uint8(len(videoTracks))
			videoTracks = append(videoTracks, track)

		case *format.Opus, *format.MPEG4Audio, *format.MPEG4AudioLATM,
			*format.MPEG1Audio, *format.AC3, *format.G711, *format.LPCM:
			w.audioTrackToID[track] = uint8(len(audioTracks))
			audioTracks = append(audioTracks, track)

		default:
			return fmt.Errorf("unsupported track: %T", track)
		}
	}

	metadata := amf0.Object{
		{
			Key: "videocodecid",
			Value: func() float64 {
				if len(videoTracks) != 0 {
					return codecID(videoTracks[0])
				}
				return 0
			}(),
		},
		{
			Key:   "videodatarate",
			Value: float64(0),
		},
		{
			Key: "audiocodecid",
			Value: func() float64 {
				if len(audioTracks) != 0 {
					return codecID(audioTracks[0])
				}
				return 0
			}(),
		},
		{
			Key:   "audiodatarate",
			Value: float64(0),
		},
	}

	if len(videoTracks) > 1 {
		var val amf0.Object

		for id, track := range videoTracks[1:] {
			val = append(val, amf0.ObjectEntry{
				Key: strconv.FormatInt(int64(id+2), 10),
				Value: amf0.Object{
					{
						Key:   "videocodecid",
						Value: codecID(track),
					},
					{
						Key:   "videodatarate",
						Value: float64(0),
					},
				},
			})
		}

		metadata = append(metadata, amf0.ObjectEntry{
			Key:   "videoTrackIdInfoMap",
			Value: val,
		})
	}

	if len(audioTracks) > 1 {
		var val amf0.Object

		for id, track := range audioTracks[1:] {
			val = append(val, amf0.ObjectEntry{
				Key: strconv.FormatInt(int64(id+2), 10),
				Value: amf0.Object{
					{
						Key:   "audiocodecid",
						Value: codecID(track),
					},
					{
						Key:   "audiodatarate",
						Value: float64(0),
					},
				},
			})
		}

		metadata = append(metadata, amf0.ObjectEntry{
			Key:   "audioTrackIdInfoMap",
			Value: val,
		})
	}

	err := w.Conn.Write(&message.DataAMF0{
		ChunkStreamID:   4,
		MessageStreamID: 0x1000000,
		Payload: []interface{}{
			"@setDataFrame",
			"onMetaData",
			metadata,
		},
	})
	if err != nil {
		return err
	}

	for id, track := range videoTracks {
		switch track := track.(type) {
		case *format.AV1:
			// TODO: fill properly.
			// unfortunately, AV1 config is not available at this stage.
			var msg message.Message = &message.VideoExSequenceStart{
				ChunkStreamID:   message.VideoChunkStreamID,
				MessageStreamID: 0x1000000,
				FourCC:          message.FourCCAV1,
				AV1Header: &mp4.Av1C{
					Marker:             0x1,
					Version:            0x1,
					SeqLevelIdx0:       0x8,
					ChromaSubsamplingX: 0x1,
					ChromaSubsamplingY: 0x1,
					ConfigOBUs:         []uint8{0xa, 0xb, 0x0, 0x0, 0x0, 0x42, 0xab, 0xbf, 0xc3, 0x70, 0xb, 0xe0, 0x1},
				},
			}

			if id != 0 {
				msg = &message.VideoExMultitrack{
					MultitrackType: 0x0,
					TrackID:        uint8(id),
					Wrapped:        msg,
				}
			}

			err = w.Conn.Write(msg)
			if err != nil {
				return err
			}

		case *format.VP9:
			// TODO: fill properly.
			// unfortunately, VP9 config is not available at this stage.
			var msg message.Message = &message.VideoExSequenceStart{
				ChunkStreamID:   message.VideoChunkStreamID,
				MessageStreamID: 0x1000000,
				FourCC:          message.FourCCVP9,
				VP9Header: &mp4.VpcC{
					FullBox:                 mp4.FullBox{Version: 0x1},
					Level:                   0x28,
					BitDepth:                0x8,
					ChromaSubsampling:       0x1,
					ColourPrimaries:         0x2,
					TransferCharacteristics: 0x2,
					MatrixCoefficients:      0x2,
					CodecInitializationData: []uint8{},
				},
			}

			if id != 0 {
				msg = &message.VideoExMultitrack{
					MultitrackType: 0x0,
					TrackID:        uint8(id),
					Wrapped:        msg,
				}
			}

			err = w.Conn.Write(msg)
			if err != nil {
				return err
			}

		case *format.H265:
			vps, sps, pps := track.SafeParams()
			if vps == nil || sps == nil || pps == nil {
				vps = formatprocessor.H265DefaultVPS
				sps = formatprocessor.H265DefaultSPS
				pps = formatprocessor.H265DefaultPPS
			}

			var msg message.Message = &message.VideoExSequenceStart{
				ChunkStreamID:   message.VideoChunkStreamID,
				MessageStreamID: 0x1000000,
				FourCC:          message.FourCCHEVC,
				HEVCHeader:      generateHvcC(vps, sps, pps),
			}

			if id != 0 {
				msg = &message.VideoExMultitrack{
					MultitrackType: 0x0,
					TrackID:        uint8(id),
					Wrapped:        msg,
				}
			}

			err = w.Conn.Write(msg)
			if err != nil {
				return err
			}

		case *format.H264:
			sps, pps := track.SafeParams()
			if sps == nil || pps == nil {
				sps = formatprocessor.H264DefaultSPS
				pps = formatprocessor.H264DefaultPPS
			}

			if id == 0 {
				buf, _ := h264conf.Conf{
					SPS: sps,
					PPS: pps,
				}.Marshal()

				err = w.Conn.Write(&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload:         buf,
				})
				if err != nil {
					return err
				}
			} else {
				err = w.Conn.Write(&message.VideoExMultitrack{
					MultitrackType: 0x0,
					TrackID:        uint8(id),
					Wrapped: &message.VideoExSequenceStart{
						ChunkStreamID:   message.VideoChunkStreamID,
						MessageStreamID: 0x1000000,
						FourCC:          message.FourCCAVC,
						AVCHeader:       generateAvcC(sps, pps),
					},
				})
				if err != nil {
					return err
				}
			}
		}
	}

	for id, track := range audioTracks {
		switch track := track.(type) {
		case *format.Opus:
			var msg message.Message = &message.AudioExSequenceStart{
				ChunkStreamID:   message.AudioChunkStreamID,
				MessageStreamID: 0x1000000,
				FourCC:          message.FourCCOpus,
				OpusHeader: &message.OpusIDHeader{
					Version:      0x1,
					ChannelCount: uint8(track.ChannelCount),
					PreSkip:      3840,
				},
			}

			if id != 0 {
				msg = &message.AudioExMultitrack{
					MultitrackType: 0x0,
					TrackID:        uint8(id),
					Wrapped:        msg,
				}
			}

			err = w.Conn.Write(msg)
			if err != nil {
				return err
			}

		case *format.MPEG4Audio:
			audioConf := track.Config

			if id == 0 {
				var enc []byte
				enc, err = audioConf.Marshal()
				if err != nil {
					return err
				}

				err = w.Conn.Write(&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        true,
					AACType:         message.AudioAACTypeConfig,
					Payload:         enc,
				})
				if err != nil {
					return err
				}
			} else {
				err = w.Conn.Write(&message.AudioExMultitrack{
					MultitrackType: 0x0,
					TrackID:        uint8(id),
					Wrapped: &message.AudioExSequenceStart{
						ChunkStreamID:   message.VideoChunkStreamID,
						MessageStreamID: 0x1000000,
						FourCC:          message.FourCCMP4A,
						AACHeader:       audioConf,
					},
				})
				if err != nil {
					return err
				}
			}

		case *format.MPEG4AudioLATM:
			audioConf := track.StreamMuxConfig.Programs[0].Layers[0].AudioSpecificConfig

			if id == 0 {
				var enc []byte
				enc, err = audioConf.Marshal()
				if err != nil {
					return err
				}

				err = w.Conn.Write(&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        true,
					AACType:         message.AudioAACTypeConfig,
					Payload:         enc,
				})
				if err != nil {
					return err
				}
			} else {
				err = w.Conn.Write(&message.AudioExMultitrack{
					MultitrackType: 0x0,
					TrackID:        uint8(id),
					Wrapped: &message.AudioExSequenceStart{
						ChunkStreamID:   message.VideoChunkStreamID,
						MessageStreamID: 0x1000000,
						FourCC:          message.FourCCMP4A,
						AACHeader:       audioConf,
					},
				})
				if err != nil {
					return err
				}
			}

		case *format.AC3:
			var msg message.Message = &message.AudioExSequenceStart{
				ChunkStreamID:   message.AudioChunkStreamID,
				MessageStreamID: 0x1000000,
				FourCC:          message.FourCCAC3,
			}

			if id != 0 {
				msg = &message.AudioExMultitrack{
					MultitrackType: 0x0,
					TrackID:        uint8(id),
					Wrapped:        msg,
				}
			}

			err = w.Conn.Write(msg)
			if err != nil {
				return err
			}

		case *format.G711:
			if id != 0 {
				return fmt.Errorf("it is not possible to use G711 tracks as secondary tracks")
			}

		case *format.LPCM:
			if id != 0 {
				return fmt.Errorf("it is not possible to use LPCM tracks as secondary tracks")
			}
		}
	}

	return nil
}

// WriteAV1 writes a AV1 temporal unit.
func (w *Writer) WriteAV1(track *format.AV1, pts time.Duration, dts time.Duration, tu [][]byte) error {
	// FFmpeg requires this.
	tu = append([][]byte{{byte(av1.OBUTypeTemporalDelimiter << 3)}}, tu...)

	enc, err := av1.Bitstream(tu).Marshal()
	if err != nil {
		return err
	}

	var msg message.Message

	if pts == dts {
		msg = &message.VideoExFramesX{
			ChunkStreamID:   message.VideoChunkStreamID,
			MessageStreamID: 0x1000000,
			FourCC:          message.FourCCAV1,
			DTS:             dts,
			Payload:         enc,
		}
	} else {
		msg = &message.VideoExCodedFrames{
			ChunkStreamID:   message.VideoChunkStreamID,
			MessageStreamID: 0x1000000,
			FourCC:          message.FourCCAV1,
			DTS:             dts,
			PTSDelta:        pts - dts,
			Payload:         enc,
		}
	}

	id := w.videoTrackToID[track]

	if id != 0 {
		msg = &message.VideoExMultitrack{
			MultitrackType: 0x0,
			TrackID:        id,
			Wrapped:        msg,
		}
	}

	return w.Conn.Write(msg)
}

// WriteVP9 writes a VP9 frame.
func (w *Writer) WriteVP9(track *format.VP9, pts time.Duration, dts time.Duration, frame []byte) error {
	var msg message.Message

	if pts == dts {
		msg = &message.VideoExFramesX{
			ChunkStreamID:   message.VideoChunkStreamID,
			MessageStreamID: 0x1000000,
			FourCC:          message.FourCCVP9,
			DTS:             dts,
			Payload:         frame,
		}
	} else {
		msg = &message.VideoExCodedFrames{
			ChunkStreamID:   message.VideoChunkStreamID,
			MessageStreamID: 0x1000000,
			FourCC:          message.FourCCVP9,
			DTS:             dts,
			PTSDelta:        pts - dts,
			Payload:         frame,
		}
	}

	id := w.videoTrackToID[track]

	if id != 0 {
		msg = &message.VideoExMultitrack{
			MultitrackType: 0x0,
			TrackID:        id,
			Wrapped:        msg,
		}
	}

	return w.Conn.Write(msg)
}

// WriteH265 writes a H265 access unit.
func (w *Writer) WriteH265(track *format.H265, pts time.Duration, dts time.Duration, au [][]byte) error {
	avcc, err := h264.AVCC(au).Marshal()
	if err != nil {
		return err
	}

	var msg message.Message

	if pts == dts {
		msg = &message.VideoExFramesX{
			ChunkStreamID:   message.VideoChunkStreamID,
			MessageStreamID: 0x1000000,
			FourCC:          message.FourCCHEVC,
			DTS:             dts,
			Payload:         avcc,
		}
	} else {
		msg = &message.VideoExCodedFrames{
			ChunkStreamID:   message.VideoChunkStreamID,
			MessageStreamID: 0x1000000,
			FourCC:          message.FourCCHEVC,
			DTS:             dts,
			PTSDelta:        pts - dts,
			Payload:         avcc,
		}
	}

	id := w.videoTrackToID[track]

	if id != 0 {
		msg = &message.VideoExMultitrack{
			MultitrackType: 0x0,
			TrackID:        id,
			Wrapped:        msg,
		}
	}

	return w.Conn.Write(msg)
}

// WriteH264 writes a H264 access unit.
func (w *Writer) WriteH264(track *format.H264, pts time.Duration, dts time.Duration, au [][]byte) error {
	avcc, err := h264.AVCC(au).Marshal()
	if err != nil {
		return err
	}

	id := w.videoTrackToID[track]

	if id == 0 {
		return w.Conn.Write(&message.Video{
			ChunkStreamID:   message.VideoChunkStreamID,
			MessageStreamID: 0x1000000,
			Codec:           message.CodecH264,
			IsKeyFrame:      h264.IsRandomAccess(au),
			Type:            message.VideoTypeAU,
			Payload:         avcc,
			DTS:             dts,
			PTSDelta:        pts - dts,
		})
	}

	var msg message.Message

	if pts == dts {
		msg = &message.VideoExFramesX{
			ChunkStreamID:   message.VideoChunkStreamID,
			MessageStreamID: 0x1000000,
			FourCC:          message.FourCCAVC,
			DTS:             dts,
			Payload:         avcc,
		}
	} else {
		msg = &message.VideoExCodedFrames{
			ChunkStreamID:   message.VideoChunkStreamID,
			MessageStreamID: 0x1000000,
			FourCC:          message.FourCCAVC,
			DTS:             dts,
			PTSDelta:        pts - dts,
			Payload:         avcc,
		}
	}

	return w.Conn.Write(&message.VideoExMultitrack{
		MultitrackType: 0x0,
		TrackID:        id,
		Wrapped:        msg,
	})
}

// WriteOpus writes a Opus packet.
func (w *Writer) WriteOpus(track *format.Opus, pts time.Duration, pkt []byte) error {
	var msg message.Message = &message.AudioExCodedFrames{
		ChunkStreamID:   message.AudioChunkStreamID,
		MessageStreamID: 0x1000000,
		DTS:             pts,
		FourCC:          message.FourCCOpus,
		Payload:         pkt,
	}

	id := w.audioTrackToID[track]

	if id != 0 {
		msg = &message.AudioExMultitrack{
			MultitrackType: 0x0,
			TrackID:        id,
			Wrapped:        msg,
		}
	}

	return w.Conn.Write(msg)
}

// WriteMPEG4Audio writes a MPEG-4 Audio access unit.
func (w *Writer) WriteMPEG4Audio(track format.Format, pts time.Duration, au []byte) error {
	id := w.audioTrackToID[track]

	if id == 0 {
		return w.Conn.Write(&message.Audio{
			ChunkStreamID:   message.AudioChunkStreamID,
			MessageStreamID: 0x1000000,
			Codec:           message.CodecMPEG4Audio,
			Rate:            message.Rate44100,
			Depth:           message.Depth16,
			IsStereo:        true,
			AACType:         message.AudioAACTypeAU,
			Payload:         au,
			DTS:             pts,
		})
	}

	return w.Conn.Write(&message.AudioExMultitrack{
		MultitrackType: 0x0,
		TrackID:        id,
		Wrapped: &message.AudioExCodedFrames{
			ChunkStreamID:   message.AudioChunkStreamID,
			MessageStreamID: 0x1000000,
			DTS:             pts,
			FourCC:          message.FourCCMP4A,
			Payload:         au,
		},
	})
}

// WriteMPEG1Audio writes a MPEG-1 Audio frame.
func (w *Writer) WriteMPEG1Audio(track *format.MPEG1Audio, pts time.Duration,
	h *mpeg1audio.FrameHeader, frame []byte,
) error {
	id := w.audioTrackToID[track]

	if id == 0 {
		rate, ok := audioRateIntToRTMP(h.SampleRate)
		if !ok {
			return fmt.Errorf("unsupported sample rate: %v", h.SampleRate)
		}

		return w.Conn.Write(&message.Audio{
			ChunkStreamID:   message.AudioChunkStreamID,
			MessageStreamID: 0x1000000,
			Codec:           message.CodecMPEG1Audio,
			Rate:            rate,
			Depth:           message.Depth16,
			IsStereo:        mpeg1AudioChannels(h.ChannelMode),
			Payload:         frame,
			DTS:             pts,
		})
	}

	return w.Conn.Write(&message.AudioExMultitrack{
		MultitrackType: 0x0,
		TrackID:        id,
		Wrapped: &message.AudioExCodedFrames{
			ChunkStreamID:   message.AudioChunkStreamID,
			MessageStreamID: 0x1000000,
			DTS:             pts,
			FourCC:          message.FourCCMP3,
			Payload:         frame,
		},
	})
}

// WriteAC3 writes an AC-3 frame.
func (w *Writer) WriteAC3(track *format.AC3, pts time.Duration, frame []byte) error {
	var msg message.Message = &message.AudioExCodedFrames{
		ChunkStreamID:   message.AudioChunkStreamID,
		MessageStreamID: 0x1000000,
		FourCC:          message.FourCCAC3,
		Payload:         frame,
		DTS:             pts,
	}

	id := w.audioTrackToID[track]

	if id != 0 {
		msg = &message.AudioExMultitrack{
			MultitrackType: 0x0,
			TrackID:        id,
			Wrapped:        msg,
		}
	}

	return w.Conn.Write(msg)
}

// WriteG711 writes G711 samples.
func (w *Writer) WriteG711(track *format.G711, pts time.Duration, samples []byte) error {
	var codec uint8

	if track.MULaw {
		codec = message.CodecPCMU
	} else {
		codec = message.CodecPCMA
	}

	return w.Conn.Write(&message.Audio{
		ChunkStreamID:   message.AudioChunkStreamID,
		MessageStreamID: 0x1000000,
		Codec:           codec,
		Rate:            message.Rate5512,
		Depth:           message.Depth16,
		IsStereo:        track.ChannelCount == 2,
		Payload:         samples,
		DTS:             pts,
	})
}

// WriteLPCM writes LPCM samples.
func (w *Writer) WriteLPCM(track *format.LPCM, pts time.Duration, samples []byte) error {
	rate, ok := audioRateIntToRTMP(track.SampleRate)
	if !ok {
		return fmt.Errorf("unsupported sample rate: %v", track.SampleRate)
	}

	le := len(samples)
	if le%2 != 0 {
		return fmt.Errorf("invalid payload length: %d", le)
	}

	samplesCopy := append([]byte(nil), samples...)

	// convert from big endian to little endian
	for i := 0; i < le; i += 2 {
		samplesCopy[i], samplesCopy[i+1] = samplesCopy[i+1], samplesCopy[i]
	}

	return w.Conn.Write(&message.Audio{
		ChunkStreamID:   message.AudioChunkStreamID,
		MessageStreamID: 0x1000000,
		Codec:           message.CodecLPCM,
		Rate:            rate,
		Depth:           message.Depth16,
		IsStereo:        (track.ChannelCount == 2),
		Payload:         samplesCopy,
		DTS:             pts,
	})
}
