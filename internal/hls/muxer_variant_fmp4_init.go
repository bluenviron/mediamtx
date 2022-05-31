package hls

import (
	"github.com/abema/go-mp4"
	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/aac"
	"github.com/aler9/gortsplib/pkg/h264"
)

type myEsds struct {
	mp4.FullBox `mp4:"0,extend"`
	Data        []byte `mp4:"1,size=8"`
}

func (*myEsds) GetType() mp4.BoxType {
	return mp4.StrToBoxType("esds")
}

func init() { //nolint:gochecknoinits
	mp4.AddBoxDef(&myEsds{}, 0)
}

func mp4InitGenerateVideoTrack(w *mp4Writer, trackID int, videoTrack *gortsplib.TrackH264) error {
	/*
		trak
		- tkhd
		- mdia
		  - mdhd
		  - hdlr
		  - minf
		    - vmhd
			- dinf
			  - dref
			    - url
			- stbl
			  - stsd
			    - avc1
				  - avcC
				  - pasp
				  - btrt
			  - stts
			  - stsc
			  - stsz
			  - stco
	*/

	_, err := w.writeBoxStart(&mp4.Trak{}) // <trak>
	if err != nil {
		return err
	}

	sps := videoTrack.SPS()
	pps := videoTrack.PPS()

	var spsp h264.SPS
	err = spsp.Unmarshal(sps)
	if err != nil {
		return err
	}

	width := spsp.Width()
	height := spsp.Height()

	_, err = w.writeBox(&mp4.Tkhd{ // <tkhd/>
		FullBox: mp4.FullBox{
			Flags: [3]byte{0, 0, 3},
		},
		TrackID: uint32(trackID),
		Width:   uint32(width * 65536),
		Height:  uint32(height * 65536),
		Matrix:  [9]int32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
	})
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(&mp4.Mdia{}) // <mdia>
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Mdhd{ // <mdhd/>
		Timescale: fmp4VideoTimescale, // the number of time units that pass per second
		Language:  [3]byte{'u', 'n', 'd'},
	})
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Hdlr{ // <hdlr/>
		HandlerType: [4]byte{'v', 'i', 'd', 'e'},
		Name:        "VideoHandler",
	})
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(&mp4.Minf{}) // <minf>
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Vmhd{ // <vmhd/>
		FullBox: mp4.FullBox{
			Flags: [3]byte{0, 0, 1},
		},
	})
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(&mp4.Dinf{}) // <dinf>
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(&mp4.Dref{ // <dref>
		EntryCount: 1,
	})
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Url{ // <url/>
		FullBox: mp4.FullBox{
			Flags: [3]byte{0, 0, 1},
		},
	})
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </dref>
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </dinf>
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(&mp4.Stbl{}) // <stbl>
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(&mp4.Stsd{ // <stsd>
		EntryCount: 1,
	})
	if err != nil {
		return err
	}

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
		return err
	}

	_, err = w.writeBox(&mp4.AVCDecoderConfiguration{ // <avcc/>
		AnyTypeBox: mp4.AnyTypeBox{
			Type: mp4.BoxTypeAvcC(),
		},
		ConfigurationVersion:       1,
		Profile:                    spsp.ProfileIdc,
		ProfileCompatibility:       sps[2],
		Level:                      spsp.LevelIdc,
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
	})
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Btrt{ // <btrt/>
		MaxBitrate: 1000000,
		AvgBitrate: 1000000,
	})
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </avc1>
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </stsd>
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Stts{ // <stts>
	})
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Stsc{ // <stsc>
	})
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Stsz{ // <stsz>
	})
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Stco{ // <stco>
	})
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </stbl>
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </minf>
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </mdia>
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </trak>
	if err != nil {
		return err
	}

	return nil
}

func mp4InitGenerateAudioTrack(w *mp4Writer, trackID int, audioTrack *gortsplib.TrackAAC) error {
	/*
		trak
		- tkhd
		- mdia
		  - mdhd
		  - hdlr
		  - minf
		    - smhd
		    - dinf
			  - dref
			    - url
		    - stbl
			  - stsd
			    - mp4a
				  - esds
				  - btrt
			  - stts
			  - stsc
			  - stsz
			  - stco
	*/

	_, err := w.writeBoxStart(&mp4.Trak{}) // <trak>
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Tkhd{ // <tkhd/>
		FullBox: mp4.FullBox{
			Flags: [3]byte{0, 0, 3},
		},
		TrackID:        uint32(trackID),
		AlternateGroup: 1,
		Volume:         256,
		Matrix:         [9]int32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
	})
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(&mp4.Mdia{}) // <mdia>
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Mdhd{ // <mdhd/>
		Timescale: uint32(audioTrack.ClockRate()),
		Language:  [3]byte{'u', 'n', 'd'},
	})
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Hdlr{ // <hdlr/>
		HandlerType: [4]byte{'s', 'o', 'u', 'n'},
		Name:        "SoundHandler",
	})
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(&mp4.Minf{}) // <minf>
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Smhd{ // <smhd/>
	})
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(&mp4.Dinf{}) // <dinf>
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(&mp4.Dref{ // <dref>
		EntryCount: 1,
	})
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Url{ // <url/>
		FullBox: mp4.FullBox{
			Flags: [3]byte{0, 0, 1},
		},
	})
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </dref>
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </dinf>
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(&mp4.Stbl{}) // <stbl>
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(&mp4.Stsd{ // <stsd>
		EntryCount: 1,
	})
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(&mp4.AudioSampleEntry{ // <mp4a>
		SampleEntry: mp4.SampleEntry{
			AnyTypeBox: mp4.AnyTypeBox{
				Type: mp4.BoxTypeMp4a(),
			},
			DataReferenceIndex: 1,
		},
		ChannelCount: uint16(audioTrack.ChannelCount()),
		SampleSize:   16,
		SampleRate:   uint32(audioTrack.ClockRate() * 65536),
	})
	if err != nil {
		return err
	}

	c := aac.MPEG4AudioConfig{
		Type:              aac.MPEG4AudioType(audioTrack.Type()),
		SampleRate:        audioTrack.ClockRate(),
		ChannelCount:      audioTrack.ChannelCount(),
		AOTSpecificConfig: audioTrack.AOTSpecificConfig(),
	}
	conf, _ := c.Encode()

	decSpecificInfoTagSize := uint8(len(conf))
	decSpecificInfoTag := append(
		[]byte{
			mp4.DecSpecificInfoTag,
			0x80, 0x80, 0x80, decSpecificInfoTagSize, // size
		},
		conf...,
	)

	esDescrTag := []byte{
		mp4.ESDescrTag,
		0x80, 0x80, 0x80, 32 + decSpecificInfoTagSize, // size
		0x00,
		byte(trackID), // ES_ID
		0x00,
	}

	decoderConfigDescrTag := []byte{
		mp4.DecoderConfigDescrTag,
		0x80, 0x80, 0x80, 18 + decSpecificInfoTagSize, // size
		0x40, // object type indicator (MPEG-4 Audio)
		0x15, 0x00,
		0x00, 0x00, 0x00, 0x01,
		0xf7, 0x39, 0x00, 0x01,
		0xf7, 0x39,
	}

	slConfigDescrTag := []byte{
		mp4.SLConfigDescrTag,
		0x80, 0x80, 0x80, 0x01, // size (1)
		0x02,
	}

	data := make([]byte, len(esDescrTag)+len(decoderConfigDescrTag)+len(decSpecificInfoTag)+len(slConfigDescrTag))
	pos := 0

	pos += copy(data[pos:], esDescrTag)
	pos += copy(data[pos:], decoderConfigDescrTag)
	pos += copy(data[pos:], decSpecificInfoTag)
	copy(data[pos:], slConfigDescrTag)

	_, err = w.writeBox(&myEsds{ // <esds/>
		Data: data,
	})
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Btrt{ // <btrt/>
		MaxBitrate: 128825,
		AvgBitrate: 128825,
	})
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </mp4a>
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </stsd>
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Stts{ // <stts>
	})
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Stsc{ // <stsc>
	})
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Stsz{ // <stsz>
	})
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Stco{ // <stco>
	})
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </stbl>
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </minf>
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </mdia>
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </trak>
	if err != nil {
		return err
	}

	return nil
}

func mp4InitGenerate(videoTrack *gortsplib.TrackH264, audioTrack *gortsplib.TrackAAC) ([]byte, error) {
	/*
		- ftyp
		- moov
		  - mvhd
		  - trak (video)
		  - trak (audio)
		- mvex
		  - trex (video)
		  - trex (audio)
	*/

	w := newMP4Writer()

	_, err := w.writeBox(&mp4.Ftyp{ // <ftyp/>
		MajorBrand:   [4]byte{'m', 'p', '4', '2'},
		MinorVersion: 1,
		CompatibleBrands: []mp4.CompatibleBrandElem{
			{CompatibleBrand: [4]byte{'m', 'p', '4', '1'}},
			{CompatibleBrand: [4]byte{'m', 'p', '4', '2'}},
			{CompatibleBrand: [4]byte{'i', 's', 'o', 'm'}},
			{CompatibleBrand: [4]byte{'h', 'l', 's', 'f'}},
		},
	})
	if err != nil {
		return nil, err
	}

	_, err = w.writeBoxStart(&mp4.Moov{}) // <moov>
	if err != nil {
		return nil, err
	}

	_, err = w.writeBox(&mp4.Mvhd{ // <mvhd/>
		Timescale:   1000,
		Rate:        65536,
		Volume:      256,
		Matrix:      [9]int32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
		NextTrackID: 2,
	})
	if err != nil {
		return nil, err
	}

	trackID := 1

	if videoTrack != nil {
		err := mp4InitGenerateVideoTrack(w, trackID, videoTrack)
		if err != nil {
			return nil, err
		}

		trackID++
	}

	if audioTrack != nil {
		err := mp4InitGenerateAudioTrack(w, trackID, audioTrack)
		if err != nil {
			return nil, err
		}
	}

	_, err = w.writeBoxStart(&mp4.Mvex{}) // <mvex>
	if err != nil {
		return nil, err
	}

	trackID = 1

	if videoTrack != nil {
		_, err = w.writeBox(&mp4.Trex{ // <trex/>
			TrackID:                       uint32(trackID),
			DefaultSampleDescriptionIndex: 1,
		})
		if err != nil {
			return nil, err
		}

		trackID++
	}

	if audioTrack != nil {
		_, err = w.writeBox(&mp4.Trex{ // <trex/>
			TrackID:                       uint32(trackID),
			DefaultSampleDescriptionIndex: 1,
		})
		if err != nil {
			return nil, err
		}
	}

	err = w.writeBoxEnd() // </mvex>
	if err != nil {
		return nil, err
	}

	err = w.writeBoxEnd() // </moov>
	if err != nil {
		return nil, err
	}

	return w.bytes(), nil
}
