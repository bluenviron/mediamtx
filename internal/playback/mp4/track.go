package mp4

import (
	"fmt"

	"github.com/abema/go-mp4"
	"github.com/bluenviron/mediacommon/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
)

// Specification: ISO 14496-1, Table 5
const (
	objectTypeIndicationVisualISO14496part2    = 0x20
	objectTypeIndicationAudioISO14496part3     = 0x40
	objectTypeIndicationVisualISO1318part2Main = 0x61
	objectTypeIndicationAudioISO11172part3     = 0x6B
	objectTypeIndicationVisualISO10918part1    = 0x6C
)

// Specification: ISO 14496-1, Table 6
const (
	streamTypeVisualStream = 0x04
	streamTypeAudioStream  = 0x05
)

func boolToUint8(v bool) uint8 {
	if v {
		return 1
	}
	return 0
}

func allSamplesAreSync(samples []*Sample) bool {
	for _, sa := range samples {
		if sa.IsNonSyncSample {
			return false
		}
	}
	return true
}

type headerTrackMarshalResult struct {
	stco                 *mp4.Stco
	stcoOffset           int
	presentationDuration uint32
}

// Track is a track of a Presentation.
type Track struct {
	ID         int
	TimeScale  uint32
	TimeOffset int32
	Codec      fmp4.Codec
	Samples    []*Sample
}

func (t *Track) marshal(w *mp4Writer) (*headerTrackMarshalResult, error) {
	/*
		|trak|
		|    |tkhd|
		|    |edts|
		|    |    |elst|
		|    |mdia|
		|    |    |mdhd|
		|    |    |hdlr|
		|    |    |minf|
		|    |    |    |vmhd| (video)
		|    |    |    |smhd| (audio)
		|    |    |    |dinf|
		|    |    |    |    |dref|
		|    |    |    |    |    |url|
		|    |    |    |stbl|
		|    |    |    |    |stsd|
		|    |    |    |    |    |av01| (AV1)
		|    |    |    |    |    |    |av1C|
		|    |    |    |    |    |vp09| (VP9)
		|    |    |    |    |    |    |vpcC|
		|    |    |    |    |    |hev1| (H265)
		|    |    |    |    |    |    |hvcC|
		|    |    |    |    |    |avc1| (H264)
		|    |    |    |    |    |    |avcC|
		|    |    |    |    |    |mp4v| (MPEG-4/2/1 video, MJPEG)
		|    |    |    |    |    |    |esds|
		|    |    |    |    |    |Opus| (Opus)
		|    |    |    |    |    |    |dOps|
		|    |    |    |    |    |mp4a| (MPEG-4/1 audio)
		|    |    |    |    |    |    |esds|
		|    |    |    |    |    |ac-3| (AC-3)
		|    |    |    |    |    |    |dac3|
		|    |    |    |    |    |ipcm| (LPCM)
		|    |    |    |    |    |    |pcmC|
		|    |    |    |    |stts|
		|    |    |    |    |stss|
		|    |    |    |    |ctts|
		|    |    |    |    |stsc|
		|    |    |    |    |stsz|
		|    |    |    |    |stco|
	*/

	_, err := w.writeBoxStart(&mp4.Trak{}) // <trak>
	if err != nil {
		return nil, err
	}

	var av1SequenceHeader *av1.SequenceHeader
	var h265SPS *h265.SPS
	var h264SPS *h264.SPS

	var width int
	var height int

	switch codec := t.Codec.(type) {
	case *fmp4.CodecAV1:
		av1SequenceHeader = &av1.SequenceHeader{}
		err = av1SequenceHeader.Unmarshal(codec.SequenceHeader)
		if err != nil {
			return nil, fmt.Errorf("unable to parse AV1 sequence header: %w", err)
		}

		width = av1SequenceHeader.Width()
		height = av1SequenceHeader.Height()

	case *fmp4.CodecVP9:
		if codec.Width == 0 {
			return nil, fmt.Errorf("VP9 parameters not provided")
		}

		width = codec.Width
		height = codec.Height

	case *fmp4.CodecH265:
		if len(codec.VPS) == 0 || len(codec.SPS) == 0 || len(codec.PPS) == 0 {
			return nil, fmt.Errorf("H265 parameters not provided")
		}

		h265SPS = &h265.SPS{}
		err = h265SPS.Unmarshal(codec.SPS)
		if err != nil {
			return nil, fmt.Errorf("unable to parse H265 SPS: %w", err)
		}

		width = h265SPS.Width()
		height = h265SPS.Height()

	case *fmp4.CodecH264:
		if len(codec.SPS) == 0 || len(codec.PPS) == 0 {
			return nil, fmt.Errorf("H264 parameters not provided")
		}

		h264SPS = &h264.SPS{}
		err = h264SPS.Unmarshal(codec.SPS)
		if err != nil {
			return nil, fmt.Errorf("unable to parse H264 SPS: %w", err)
		}

		width = h264SPS.Width()
		height = h264SPS.Height()

	case *fmp4.CodecMPEG4Video:
		if len(codec.Config) == 0 {
			return nil, fmt.Errorf("MPEG-4 Video config not provided")
		}

		// TODO: parse config and use real values
		width = 800
		height = 600

	case *fmp4.CodecMPEG1Video:
		if len(codec.Config) == 0 {
			return nil, fmt.Errorf("MPEG-1/2 Video config not provided")
		}

		// TODO: parse config and use real values
		width = 800
		height = 600

	case *fmp4.CodecMJPEG:
		if codec.Width == 0 {
			return nil, fmt.Errorf("M-JPEG parameters not provided")
		}

		width = codec.Width
		height = codec.Height
	}

	sampleDuration := uint32(0)
	for _, sa := range t.Samples {
		sampleDuration += sa.Duration
	}

	presentationDuration := uint32(((int64(sampleDuration) + int64(t.TimeOffset)) * globalTimescale) / int64(t.TimeScale))

	if t.Codec.IsVideo() {
		_, err = w.writeBox(&mp4.Tkhd{ // <tkhd/>
			FullBox: mp4.FullBox{
				Flags: [3]byte{0, 0, 3},
			},
			TrackID:    uint32(t.ID),
			DurationV0: presentationDuration,
			Width:      uint32(width * 65536),
			Height:     uint32(height * 65536),
			Matrix:     [9]int32{0x10000, 0, 0, 0, 0x10000, 0, 0, 0, 0x40000000},
		})
		if err != nil {
			return nil, err
		}
	} else {
		_, err = w.writeBox(&mp4.Tkhd{ // <tkhd/>
			FullBox: mp4.FullBox{
				Flags: [3]byte{0, 0, 3},
			},
			TrackID:        uint32(t.ID),
			DurationV0:     presentationDuration,
			AlternateGroup: 1,
			Volume:         256,
			Matrix:         [9]int32{0x10000, 0, 0, 0, 0x10000, 0, 0, 0, 0x40000000},
		})
		if err != nil {
			return nil, err
		}
	}

	_, err = w.writeBoxStart(&mp4.Edts{}) // <edts>
	if err != nil {
		return nil, err
	}

	err = t.marshalELST(w, sampleDuration) // <elst/>
	if err != nil {
		return nil, err
	}

	err = w.writeBoxEnd() // </edts>
	if err != nil {
		return nil, err
	}

	_, err = w.writeBoxStart(&mp4.Mdia{}) // <mdia>
	if err != nil {
		return nil, err
	}

	_, err = w.writeBox(&mp4.Mdhd{ // <mdhd/>
		Timescale:  t.TimeScale,
		DurationV0: uint32(int64(sampleDuration) + int64(t.TimeOffset)),
		Language:   [3]byte{'u', 'n', 'd'},
	})
	if err != nil {
		return nil, err
	}

	if t.Codec.IsVideo() {
		_, err = w.writeBox(&mp4.Hdlr{ // <hdlr/>
			HandlerType: [4]byte{'v', 'i', 'd', 'e'},
			Name:        "VideoHandler",
		})
		if err != nil {
			return nil, err
		}
	} else {
		_, err = w.writeBox(&mp4.Hdlr{ // <hdlr/>
			HandlerType: [4]byte{'s', 'o', 'u', 'n'},
			Name:        "SoundHandler",
		})
		if err != nil {
			return nil, err
		}
	}

	_, err = w.writeBoxStart(&mp4.Minf{}) // <minf>
	if err != nil {
		return nil, err
	}

	if t.Codec.IsVideo() {
		_, err = w.writeBox(&mp4.Vmhd{ // <vmhd/>
			FullBox: mp4.FullBox{
				Flags: [3]byte{0, 0, 1},
			},
		})
		if err != nil {
			return nil, err
		}
	} else {
		_, err = w.writeBox(&mp4.Smhd{}) // <smhd/>
		if err != nil {
			return nil, err
		}
	}

	_, err = w.writeBoxStart(&mp4.Dinf{}) // <dinf>
	if err != nil {
		return nil, err
	}

	_, err = w.writeBoxStart(&mp4.Dref{ // <dref>
		EntryCount: 1,
	})
	if err != nil {
		return nil, err
	}

	_, err = w.writeBox(&mp4.Url{ // <url/>
		FullBox: mp4.FullBox{
			Flags: [3]byte{0, 0, 1},
		},
	})
	if err != nil {
		return nil, err
	}

	err = w.writeBoxEnd() // </dref>
	if err != nil {
		return nil, err
	}

	err = w.writeBoxEnd() // </dinf>
	if err != nil {
		return nil, err
	}

	_, err = w.writeBoxStart(&mp4.Stbl{}) // <stbl>
	if err != nil {
		return nil, err
	}

	_, err = w.writeBoxStart(&mp4.Stsd{ // <stsd>
		EntryCount: 1,
	})
	if err != nil {
		return nil, err
	}

	switch codec := t.Codec.(type) {
	case *fmp4.CodecAV1:
		_, err = w.writeBoxStart(&mp4.VisualSampleEntry{ // <av01>
			SampleEntry: mp4.SampleEntry{
				AnyTypeBox: mp4.AnyTypeBox{
					Type: mp4.BoxTypeAv01(),
				},
				DataReferenceIndex: 1,
			},
			Width:           uint16(width),
			Height:          uint16(height),
			Horizresolution: 4718592,
			Vertresolution:  4718592,
			FrameCount:      1,
			Depth:           24,
			PreDefined3:     -1,
		})
		if err != nil {
			return nil, err
		}

		var bs []byte
		bs, err = av1.BitstreamMarshal([][]byte{codec.SequenceHeader})
		if err != nil {
			return nil, err
		}

		_, err = w.writeBox(&mp4.Av1C{ // <av1C/>
			Marker:               1,
			Version:              1,
			SeqProfile:           av1SequenceHeader.SeqProfile,
			SeqLevelIdx0:         av1SequenceHeader.SeqLevelIdx[0],
			SeqTier0:             boolToUint8(av1SequenceHeader.SeqTier[0]),
			HighBitdepth:         boolToUint8(av1SequenceHeader.ColorConfig.HighBitDepth),
			TwelveBit:            boolToUint8(av1SequenceHeader.ColorConfig.TwelveBit),
			Monochrome:           boolToUint8(av1SequenceHeader.ColorConfig.MonoChrome),
			ChromaSubsamplingX:   boolToUint8(av1SequenceHeader.ColorConfig.SubsamplingX),
			ChromaSubsamplingY:   boolToUint8(av1SequenceHeader.ColorConfig.SubsamplingY),
			ChromaSamplePosition: uint8(av1SequenceHeader.ColorConfig.ChromaSamplePosition),
			ConfigOBUs:           bs,
		})
		if err != nil {
			return nil, err
		}

	case *fmp4.CodecVP9:
		_, err = w.writeBoxStart(&mp4.VisualSampleEntry{ // <vp09>
			SampleEntry: mp4.SampleEntry{
				AnyTypeBox: mp4.AnyTypeBox{
					Type: mp4.BoxTypeVp09(),
				},
				DataReferenceIndex: 1,
			},
			Width:           uint16(width),
			Height:          uint16(height),
			Horizresolution: 4718592,
			Vertresolution:  4718592,
			FrameCount:      1,
			Depth:           24,
			PreDefined3:     -1,
		})
		if err != nil {
			return nil, err
		}

		_, err = w.writeBox(&mp4.VpcC{ // <vpcC/>
			FullBox: mp4.FullBox{
				Version: 1,
			},
			Profile:            codec.Profile,
			Level:              10, // level 1
			BitDepth:           codec.BitDepth,
			ChromaSubsampling:  codec.ChromaSubsampling,
			VideoFullRangeFlag: boolToUint8(codec.ColorRange),
		})
		if err != nil {
			return nil, err
		}

	case *fmp4.CodecH265:
		_, err = w.writeBoxStart(&mp4.VisualSampleEntry{ // <hev1>
			SampleEntry: mp4.SampleEntry{
				AnyTypeBox: mp4.AnyTypeBox{
					Type: mp4.BoxTypeHev1(),
				},
				DataReferenceIndex: 1,
			},
			Width:           uint16(width),
			Height:          uint16(height),
			Horizresolution: 4718592,
			Vertresolution:  4718592,
			FrameCount:      1,
			Depth:           24,
			PreDefined3:     -1,
		})
		if err != nil {
			return nil, err
		}

		_, err = w.writeBox(&mp4.HvcC{ // <hvcC/>
			ConfigurationVersion:        1,
			GeneralProfileIdc:           h265SPS.ProfileTierLevel.GeneralProfileIdc,
			GeneralProfileCompatibility: h265SPS.ProfileTierLevel.GeneralProfileCompatibilityFlag,
			GeneralConstraintIndicator: [6]uint8{
				codec.SPS[7], codec.SPS[8], codec.SPS[9],
				codec.SPS[10], codec.SPS[11], codec.SPS[12],
			},
			GeneralLevelIdc: h265SPS.ProfileTierLevel.GeneralLevelIdc,
			// MinSpatialSegmentationIdc
			// ParallelismType
			ChromaFormatIdc:      uint8(h265SPS.ChromaFormatIdc),
			BitDepthLumaMinus8:   uint8(h265SPS.BitDepthLumaMinus8),
			BitDepthChromaMinus8: uint8(h265SPS.BitDepthChromaMinus8),
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
						Length:  uint16(len(codec.VPS)),
						NALUnit: codec.VPS,
					}},
				},
				{
					NaluType: byte(h265.NALUType_SPS_NUT),
					NumNalus: 1,
					Nalus: []mp4.HEVCNalu{{
						Length:  uint16(len(codec.SPS)),
						NALUnit: codec.SPS,
					}},
				},
				{
					NaluType: byte(h265.NALUType_PPS_NUT),
					NumNalus: 1,
					Nalus: []mp4.HEVCNalu{{
						Length:  uint16(len(codec.PPS)),
						NALUnit: codec.PPS,
					}},
				},
			},
		})
		if err != nil {
			return nil, err
		}

	case *fmp4.CodecH264:
		_, err = w.writeBoxStart(&mp4.VisualSampleEntry{ // <avc1>
			SampleEntry: mp4.SampleEntry{
				AnyTypeBox: mp4.AnyTypeBox{
					Type: mp4.BoxTypeAvc1(),
				},
				DataReferenceIndex: 1,
			},
			Width:           uint16(width),
			Height:          uint16(height),
			Horizresolution: 4718592,
			Vertresolution:  4718592,
			FrameCount:      1,
			Depth:           24,
			PreDefined3:     -1,
		})
		if err != nil {
			return nil, err
		}

		_, err = w.writeBox(&mp4.AVCDecoderConfiguration{ // <avcc/>
			AnyTypeBox: mp4.AnyTypeBox{
				Type: mp4.BoxTypeAvcC(),
			},
			ConfigurationVersion:       1,
			Profile:                    h264SPS.ProfileIdc,
			ProfileCompatibility:       codec.SPS[2],
			Level:                      h264SPS.LevelIdc,
			LengthSizeMinusOne:         3,
			NumOfSequenceParameterSets: 1,
			SequenceParameterSets: []mp4.AVCParameterSet{
				{
					Length:  uint16(len(codec.SPS)),
					NALUnit: codec.SPS,
				},
			},
			NumOfPictureParameterSets: 1,
			PictureParameterSets: []mp4.AVCParameterSet{
				{
					Length:  uint16(len(codec.PPS)),
					NALUnit: codec.PPS,
				},
			},
		})
		if err != nil {
			return nil, err
		}

	case *fmp4.CodecMPEG4Video: //nolint:dupl
		_, err = w.writeBoxStart(&mp4.VisualSampleEntry{ // <mp4v>
			SampleEntry: mp4.SampleEntry{
				AnyTypeBox: mp4.AnyTypeBox{
					Type: mp4.BoxTypeMp4v(),
				},
				DataReferenceIndex: 1,
			},
			Width:           uint16(width),
			Height:          uint16(height),
			Horizresolution: 4718592,
			Vertresolution:  4718592,
			FrameCount:      1,
			Depth:           24,
			PreDefined3:     -1,
		})
		if err != nil {
			return nil, err
		}

		_, err = w.writeBox(&mp4.Esds{ // <esds/>
			Descriptors: []mp4.Descriptor{
				{
					Tag:  mp4.ESDescrTag,
					Size: 32 + uint32(len(codec.Config)),
					ESDescriptor: &mp4.ESDescriptor{
						ESID: uint16(t.ID),
					},
				},
				{
					Tag:  mp4.DecoderConfigDescrTag,
					Size: 18 + uint32(len(codec.Config)),
					DecoderConfigDescriptor: &mp4.DecoderConfigDescriptor{
						ObjectTypeIndication: objectTypeIndicationVisualISO14496part2,
						StreamType:           streamTypeVisualStream,
						Reserved:             true,
						MaxBitrate:           1000000,
						AvgBitrate:           1000000,
					},
				},
				{
					Tag:  mp4.DecSpecificInfoTag,
					Size: uint32(len(codec.Config)),
					Data: codec.Config,
				},
				{
					Tag:  mp4.SLConfigDescrTag,
					Size: 1,
					Data: []byte{0x02},
				},
			},
		})
		if err != nil {
			return nil, err
		}

	case *fmp4.CodecMPEG1Video: //nolint:dupl
		_, err = w.writeBoxStart(&mp4.VisualSampleEntry{ // <mp4v>
			SampleEntry: mp4.SampleEntry{
				AnyTypeBox: mp4.AnyTypeBox{
					Type: mp4.BoxTypeMp4v(),
				},
				DataReferenceIndex: 1,
			},
			Width:           uint16(width),
			Height:          uint16(height),
			Horizresolution: 4718592,
			Vertresolution:  4718592,
			FrameCount:      1,
			Depth:           24,
			PreDefined3:     -1,
		})
		if err != nil {
			return nil, err
		}

		_, err = w.writeBox(&mp4.Esds{ // <esds/>
			Descriptors: []mp4.Descriptor{
				{
					Tag:  mp4.ESDescrTag,
					Size: 32 + uint32(len(codec.Config)),
					ESDescriptor: &mp4.ESDescriptor{
						ESID: uint16(t.ID),
					},
				},
				{
					Tag:  mp4.DecoderConfigDescrTag,
					Size: 18 + uint32(len(codec.Config)),
					DecoderConfigDescriptor: &mp4.DecoderConfigDescriptor{
						ObjectTypeIndication: objectTypeIndicationVisualISO1318part2Main,
						StreamType:           streamTypeVisualStream,
						Reserved:             true,
						MaxBitrate:           1000000,
						AvgBitrate:           1000000,
					},
				},
				{
					Tag:  mp4.DecSpecificInfoTag,
					Size: uint32(len(codec.Config)),
					Data: codec.Config,
				},
				{
					Tag:  mp4.SLConfigDescrTag,
					Size: 1,
					Data: []byte{0x02},
				},
			},
		})
		if err != nil {
			return nil, err
		}

	case *fmp4.CodecMJPEG: //nolint:dupl
		_, err = w.writeBoxStart(&mp4.VisualSampleEntry{ // <mp4v>
			SampleEntry: mp4.SampleEntry{
				AnyTypeBox: mp4.AnyTypeBox{
					Type: mp4.BoxTypeMp4v(),
				},
				DataReferenceIndex: 1,
			},
			Width:           uint16(width),
			Height:          uint16(height),
			Horizresolution: 4718592,
			Vertresolution:  4718592,
			FrameCount:      1,
			Depth:           24,
			PreDefined3:     -1,
		})
		if err != nil {
			return nil, err
		}

		_, err = w.writeBox(&mp4.Esds{ // <esds/>
			Descriptors: []mp4.Descriptor{
				{
					Tag:  mp4.ESDescrTag,
					Size: 27,
					ESDescriptor: &mp4.ESDescriptor{
						ESID: uint16(t.ID),
					},
				},
				{
					Tag:  mp4.DecoderConfigDescrTag,
					Size: 13,
					DecoderConfigDescriptor: &mp4.DecoderConfigDescriptor{
						ObjectTypeIndication: objectTypeIndicationVisualISO10918part1,
						StreamType:           streamTypeVisualStream,
						Reserved:             true,
						MaxBitrate:           1000000,
						AvgBitrate:           1000000,
					},
				},
				{
					Tag:  mp4.SLConfigDescrTag,
					Size: 1,
					Data: []byte{0x02},
				},
			},
		})
		if err != nil {
			return nil, err
		}

	case *fmp4.CodecOpus:
		_, err = w.writeBoxStart(&mp4.AudioSampleEntry{ // <Opus>
			SampleEntry: mp4.SampleEntry{
				AnyTypeBox: mp4.AnyTypeBox{
					Type: mp4.BoxTypeOpus(),
				},
				DataReferenceIndex: 1,
			},
			ChannelCount: uint16(codec.ChannelCount),
			SampleSize:   16,
			SampleRate:   48000 * 65536,
		})
		if err != nil {
			return nil, err
		}

		_, err = w.writeBox(&mp4.DOps{ // <dOps/>
			OutputChannelCount: uint8(codec.ChannelCount),
			PreSkip:            312,
			InputSampleRate:    48000,
		})
		if err != nil {
			return nil, err
		}

	case *fmp4.CodecMPEG4Audio:
		_, err = w.writeBoxStart(&mp4.AudioSampleEntry{ // <mp4a>
			SampleEntry: mp4.SampleEntry{
				AnyTypeBox: mp4.AnyTypeBox{
					Type: mp4.BoxTypeMp4a(),
				},
				DataReferenceIndex: 1,
			},
			ChannelCount: uint16(codec.ChannelCount),
			SampleSize:   16,
			SampleRate:   uint32(codec.SampleRate * 65536),
		})
		if err != nil {
			return nil, err
		}

		enc, _ := codec.Config.Marshal()

		_, err = w.writeBox(&mp4.Esds{ // <esds/>
			Descriptors: []mp4.Descriptor{
				{
					Tag:  mp4.ESDescrTag,
					Size: 32 + uint32(len(enc)),
					ESDescriptor: &mp4.ESDescriptor{
						ESID: uint16(t.ID),
					},
				},
				{
					Tag:  mp4.DecoderConfigDescrTag,
					Size: 18 + uint32(len(enc)),
					DecoderConfigDescriptor: &mp4.DecoderConfigDescriptor{
						ObjectTypeIndication: objectTypeIndicationAudioISO14496part3,
						StreamType:           streamTypeAudioStream,
						Reserved:             true,
						MaxBitrate:           128825,
						AvgBitrate:           128825,
					},
				},
				{
					Tag:  mp4.DecSpecificInfoTag,
					Size: uint32(len(enc)),
					Data: enc,
				},
				{
					Tag:  mp4.SLConfigDescrTag,
					Size: 1,
					Data: []byte{0x02},
				},
			},
		})
		if err != nil {
			return nil, err
		}

	case *fmp4.CodecMPEG1Audio:
		_, err = w.writeBoxStart(&mp4.AudioSampleEntry{ // <mp4a>
			SampleEntry: mp4.SampleEntry{
				AnyTypeBox: mp4.AnyTypeBox{
					Type: mp4.BoxTypeMp4a(),
				},
				DataReferenceIndex: 1,
			},
			ChannelCount: uint16(codec.ChannelCount),
			SampleSize:   16,
			SampleRate:   uint32(codec.SampleRate * 65536),
		})
		if err != nil {
			return nil, err
		}

		_, err = w.writeBox(&mp4.Esds{ // <esds/>
			Descriptors: []mp4.Descriptor{
				{
					Tag:  mp4.ESDescrTag,
					Size: 27,
					ESDescriptor: &mp4.ESDescriptor{
						ESID: uint16(t.ID),
					},
				},
				{
					Tag:  mp4.DecoderConfigDescrTag,
					Size: 13,
					DecoderConfigDescriptor: &mp4.DecoderConfigDescriptor{
						ObjectTypeIndication: objectTypeIndicationAudioISO11172part3,
						StreamType:           streamTypeAudioStream,
						Reserved:             true,
						MaxBitrate:           128825,
						AvgBitrate:           128825,
					},
				},
				{
					Tag:  mp4.SLConfigDescrTag,
					Size: 1,
					Data: []byte{0x02},
				},
			},
		})
		if err != nil {
			return nil, err
		}

	case *fmp4.CodecAC3:
		_, err = w.writeBoxStart(&mp4.AudioSampleEntry{ // <ac-3>
			SampleEntry: mp4.SampleEntry{
				AnyTypeBox: mp4.AnyTypeBox{
					Type: mp4.BoxTypeAC3(),
				},
				DataReferenceIndex: 1,
			},
			ChannelCount: uint16(codec.ChannelCount),
			SampleSize:   16,
			SampleRate:   uint32(codec.SampleRate * 65536),
		})
		if err != nil {
			return nil, err
		}

		_, err = w.writeBox(&mp4.Dac3{ // <dac3/>
			Fscod: codec.Fscod,
			Bsid:  codec.Bsid,
			Bsmod: codec.Bsmod,
			Acmod: codec.Acmod,
			LfeOn: func() uint8 {
				if codec.LfeOn {
					return 1
				}
				return 0
			}(),
			BitRateCode: codec.BitRateCode,
		})
		if err != nil {
			return nil, err
		}

	case *fmp4.CodecLPCM:
		_, err = w.writeBoxStart(&mp4.AudioSampleEntry{ // <ipcm>
			SampleEntry: mp4.SampleEntry{
				AnyTypeBox: mp4.AnyTypeBox{
					Type: mp4.BoxTypeIpcm(),
				},
				DataReferenceIndex: 1,
			},
			ChannelCount: uint16(codec.ChannelCount),
			SampleSize:   uint16(codec.BitDepth), // FFmpeg leaves this to 16 instead of using real bit depth
			SampleRate:   uint32(codec.SampleRate * 65536),
		})
		if err != nil {
			return nil, err
		}

		_, err = w.writeBox(&mp4.PcmC{ // <pcmC/>
			FormatFlags: func() uint8 {
				if codec.LittleEndian {
					return 1
				}
				return 0
			}(),
			PCMSampleSize: uint8(codec.BitDepth),
		})
		if err != nil {
			return nil, err
		}
	}

	err = w.writeBoxEnd() // </*>
	if err != nil {
		return nil, err
	}

	err = w.writeBoxEnd() // </stsd>
	if err != nil {
		return nil, err
	}

	err = t.marshalSTTS(w) // <stts/>
	if err != nil {
		return nil, err
	}

	err = t.marshalSTSS(w) // <stss/>
	if err != nil {
		return nil, err
	}

	err = t.marshalCTTS(w) // <ctts/>
	if err != nil {
		return nil, err
	}

	err = t.marshalSTSC(w) // <stsc/>
	if err != nil {
		return nil, err
	}

	err = t.marshalSTSZ(w) // <stsz/>
	if err != nil {
		return nil, err
	}

	stco, stcoOffset, err := t.marshalSTCO(w) // <stco/>
	if err != nil {
		return nil, err
	}

	err = w.writeBoxEnd() // </stbl>
	if err != nil {
		return nil, err
	}

	err = w.writeBoxEnd() // </minf>
	if err != nil {
		return nil, err
	}

	err = w.writeBoxEnd() // </mdia>
	if err != nil {
		return nil, err
	}

	err = w.writeBoxEnd() // </trak>
	if err != nil {
		return nil, err
	}

	return &headerTrackMarshalResult{
		stco:                 stco,
		stcoOffset:           stcoOffset,
		presentationDuration: presentationDuration,
	}, nil
}

func (t *Track) marshalELST(w *mp4Writer, sampleDuration uint32) error {
	if t.TimeOffset > 0 {
		_, err := w.writeBox(&mp4.Elst{
			EntryCount: 2,
			Entries: []mp4.ElstEntry{
				{ // pause
					SegmentDurationV0: uint32((uint64(t.TimeOffset) * globalTimescale) / uint64(t.TimeScale)),
					MediaTimeV0:       -1,
					MediaRateInteger:  1,
					MediaRateFraction: 0,
				},
				{ // presentation
					SegmentDurationV0: uint32((uint64(sampleDuration) * globalTimescale) / uint64(t.TimeScale)),
					MediaTimeV0:       0,
					MediaRateInteger:  1,
					MediaRateFraction: 0,
				},
			},
		})
		return err
	}

	_, err := w.writeBox(&mp4.Elst{
		EntryCount: 1,
		Entries: []mp4.ElstEntry{{
			SegmentDurationV0: uint32(((uint64(sampleDuration) +
				uint64(-t.TimeOffset)) * globalTimescale) / uint64(t.TimeScale)),
			MediaTimeV0:       -t.TimeOffset,
			MediaRateInteger:  1,
			MediaRateFraction: 0,
		}},
	})
	return err
}

func (t *Track) marshalSTTS(w *mp4Writer) error {
	entries := []mp4.SttsEntry{{
		SampleCount: 1,
		SampleDelta: t.Samples[0].Duration,
	}}

	for _, sa := range t.Samples[1:] {
		if sa.Duration == entries[len(entries)-1].SampleDelta {
			entries[len(entries)-1].SampleCount++
		} else {
			entries = append(entries, mp4.SttsEntry{
				SampleCount: 1,
				SampleDelta: sa.Duration,
			})
		}
	}

	_, err := w.writeBox(&mp4.Stts{
		EntryCount: uint32(len(entries)),
		Entries:    entries,
	})
	return err
}

func (t *Track) marshalSTSS(w *mp4Writer) error {
	if allSamplesAreSync(t.Samples) {
		return nil
	}

	var sampleNumbers []uint32

	for i, sa := range t.Samples {
		if !sa.IsNonSyncSample {
			sampleNumbers = append(sampleNumbers, uint32(i+1))
		}
	}

	_, err := w.writeBox(&mp4.Stss{
		EntryCount:   uint32(len(sampleNumbers)),
		SampleNumber: sampleNumbers,
	})
	return err
}

func (t *Track) marshalCTTS(w *mp4Writer) error {
	entries := []mp4.CttsEntry{{
		SampleCount:    1,
		SampleOffsetV0: uint32(t.Samples[0].PTSOffset),
	}}

	for _, sa := range t.Samples[1:] {
		if uint32(sa.PTSOffset) == entries[len(entries)-1].SampleOffsetV0 {
			entries[len(entries)-1].SampleCount++
		} else {
			entries = append(entries, mp4.CttsEntry{
				SampleCount:    1,
				SampleOffsetV0: uint32(sa.PTSOffset),
			})
		}
	}

	_, err := w.writeBox(&mp4.Ctts{
		FullBox: mp4.FullBox{
			Version: 0,
		},
		EntryCount: uint32(len(entries)),
		Entries:    entries,
	})
	return err
}

func (t *Track) marshalSTSC(w *mp4Writer) error {
	entries := []mp4.StscEntry{{
		FirstChunk:             1,
		SamplesPerChunk:        1,
		SampleDescriptionIndex: 1,
	}}

	firstSample := t.Samples[0]
	off := firstSample.offset + firstSample.PayloadSize

	for _, sa := range t.Samples[1:] {
		if sa.offset == off {
			entries[len(entries)-1].SamplesPerChunk++
		} else {
			entries = append(entries, mp4.StscEntry{
				FirstChunk:             uint32(len(entries) + 1),
				SamplesPerChunk:        1,
				SampleDescriptionIndex: 1,
			})
		}

		off = sa.offset + sa.PayloadSize
	}

	// further compression
	for i := len(entries) - 1; i >= 1; i-- {
		if entries[i].SamplesPerChunk == entries[i-1].SamplesPerChunk {
			for j := i; j < len(entries)-1; j++ {
				entries[j] = entries[j+1]
			}
			entries = entries[:len(entries)-1]
		}
	}

	_, err := w.writeBox(&mp4.Stsc{
		EntryCount: uint32(len(entries)),
		Entries:    entries,
	})
	return err
}

func (t *Track) marshalSTSZ(w *mp4Writer) error {
	sampleSizes := make([]uint32, len(t.Samples))

	for i, sa := range t.Samples {
		sampleSizes[i] = sa.PayloadSize
	}

	_, err := w.writeBox(&mp4.Stsz{
		SampleSize:  0,
		SampleCount: uint32(len(sampleSizes)),
		EntrySize:   sampleSizes,
	})
	return err
}

func (t *Track) marshalSTCO(w *mp4Writer) (*mp4.Stco, int, error) {
	firstSample := t.Samples[0]
	off := firstSample.offset + firstSample.PayloadSize

	entries := []uint32{firstSample.offset}

	for _, sa := range t.Samples[1:] {
		if sa.offset != off {
			entries = append(entries, sa.offset)
		}
		off = sa.offset + sa.PayloadSize
	}

	stco := &mp4.Stco{
		EntryCount:  uint32(len(entries)),
		ChunkOffset: entries,
	}

	offset, err := w.writeBox(stco)
	if err != nil {
		return nil, 0, err
	}

	return stco, offset, err
}
