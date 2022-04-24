package hls

import (
	"github.com/abema/go-mp4"
	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
)

func mp4InitGenerate(videoTrack *gortsplib.TrackH264, audioTrack *gortsplib.TrackAAC) ([]byte, error) {
	/*
			ftyp
			moov
			- mvhd
			- trak
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
			- mvex
			  - trex
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

	_, err = w.writeBoxStart(&mp4.Trak{}) // <trak>
	if err != nil {
		return nil, err
	}

	sps := videoTrack.SPS()
	pps := videoTrack.PPS()

	var spsp h264.SPS
	err = spsp.Unmarshal(sps)
	if err != nil {
		return nil, err
	}

	width := spsp.Width()
	height := spsp.Height()

	_, err = w.writeBox(&mp4.Tkhd{ // <tkhd/>
		FullBox: mp4.FullBox{
			Flags: [3]byte{0, 0, 3},
		},
		TrackID: 1,
		Width:   uint32(width * 65536),
		Height:  uint32(height * 65536),
		Matrix:  [9]int32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
	})
	if err != nil {
		return nil, err
	}

	_, err = w.writeBoxStart(&mp4.Mdia{}) // <mdia>
	if err != nil {
		return nil, err
	}

	_, err = w.writeBox(&mp4.Mdhd{ // <mdhd/>
		Timescale: fmp4Timescale, // the number of time units that pass per second
		Language:  [3]byte{'u', 'n', 'd'},
	})
	if err != nil {
		return nil, err
	}

	_, err = w.writeBox(&mp4.Hdlr{ // <hdlr/>
		HandlerType: [4]byte{'v', 'i', 'd', 'e'},
		Name:        "VideoHandler",
	})
	if err != nil {
		return nil, err
	}

	_, err = w.writeBoxStart(&mp4.Minf{}) // <minf>
	if err != nil {
		return nil, err
	}

	_, err = w.writeBox(&mp4.Vmhd{ // <vmhd/>
		FullBox: mp4.FullBox{
			Flags: [3]byte{0, 0, 1},
		},
	})
	if err != nil {
		return nil, err
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
		return nil, err
	}

	_, err = w.writeBox(&mp4.Btrt{ // <btrt/>
		MaxBitrate: 1000000,
		AvgBitrate: 1000000,
	})
	if err != nil {
		return nil, err
	}

	err = w.writeBoxEnd() // </avc1>
	if err != nil {
		return nil, err
	}

	err = w.writeBoxEnd() // </stsd>
	if err != nil {
		return nil, err
	}

	_, err = w.writeBox(&mp4.Stts{ // <stts>
	})
	if err != nil {
		return nil, err
	}

	_, err = w.writeBox(&mp4.Stsc{ // <stsc>
	})
	if err != nil {
		return nil, err
	}

	_, err = w.writeBox(&mp4.Stsz{ // <stsz>
	})
	if err != nil {
		return nil, err
	}

	_, err = w.writeBox(&mp4.Stco{ // <stco>
	})
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

	_, err = w.writeBoxStart(&mp4.Mvex{}) // <mvex>
	if err != nil {
		return nil, err
	}

	_, err = w.writeBox(&mp4.Trex{ // <trex/>
		TrackID:                       1,
		DefaultSampleDescriptionIndex: 1,
	})
	if err != nil {
		return nil, err
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
