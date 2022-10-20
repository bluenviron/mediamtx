package fmp4

import (
	gomp4 "github.com/abema/go-mp4"
	"github.com/aler9/gortsplib"

	"github.com/aler9/gortsplib/pkg/h264"
)

// InitTrack is a track of a FMP4 initialization file.
type InitTrack struct {
	ID        int
	TimeScale uint32
	Track     gortsplib.Track
}

func (track *InitTrack) marshal(w *mp4Writer) error {
	/*
		trak
		- tkhd
		- mdia
		  - mdhd
		  - hdlr
		  - minf
		    - vmhd (video only)
			- smhd (audio only)
			- dinf
			  - dref
			    - url
			- stbl
			  - stsd
			    - avc1 (h264 only)
				  - avcC
				  - pasp
				  - btrt
				- mp4a (mpeg4audio only)
				  - esds
				  - btrt
			  - stts
			  - stsc
			  - stsz
			  - stco
	*/

	_, err := w.writeBoxStart(&gomp4.Trak{}) // <trak>
	if err != nil {
		return err
	}

	var sps []byte
	var pps []byte
	var spsp h264.SPS
	var width int
	var height int

	switch ttrack := track.Track.(type) {
	case *gortsplib.TrackH264:
		sps = ttrack.SafeSPS()
		pps = ttrack.SafePPS()

		err = spsp.Unmarshal(sps)
		if err != nil {
			return err
		}

		width = spsp.Width()
		height = spsp.Height()

		_, err = w.WriteBox(&gomp4.Tkhd{ // <tkhd/>
			FullBox: gomp4.FullBox{
				Flags: [3]byte{0, 0, 3},
			},
			TrackID: uint32(track.ID),
			Width:   uint32(width * 65536),
			Height:  uint32(height * 65536),
			Matrix:  [9]int32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
		})
		if err != nil {
			return err
		}

	case *gortsplib.TrackMPEG4Audio:
		_, err = w.WriteBox(&gomp4.Tkhd{ // <tkhd/>
			FullBox: gomp4.FullBox{
				Flags: [3]byte{0, 0, 3},
			},
			TrackID:        uint32(track.ID),
			AlternateGroup: 1,
			Volume:         256,
			Matrix:         [9]int32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
		})
		if err != nil {
			return err
		}
	}

	_, err = w.writeBoxStart(&gomp4.Mdia{}) // <mdia>
	if err != nil {
		return err
	}

	_, err = w.WriteBox(&gomp4.Mdhd{ // <mdhd/>
		Timescale: track.TimeScale,
		Language:  [3]byte{'u', 'n', 'd'},
	})
	if err != nil {
		return err
	}

	switch track.Track.(type) {
	case *gortsplib.TrackH264:
		_, err = w.WriteBox(&gomp4.Hdlr{ // <hdlr/>
			HandlerType: [4]byte{'v', 'i', 'd', 'e'},
			Name:        "VideoHandler",
		})
		if err != nil {
			return err
		}

	case *gortsplib.TrackMPEG4Audio:
		_, err = w.WriteBox(&gomp4.Hdlr{ // <hdlr/>
			HandlerType: [4]byte{'s', 'o', 'u', 'n'},
			Name:        "SoundHandler",
		})
		if err != nil {
			return err
		}
	}

	_, err = w.writeBoxStart(&gomp4.Minf{}) // <minf>
	if err != nil {
		return err
	}

	switch track.Track.(type) {
	case *gortsplib.TrackH264:
		_, err = w.WriteBox(&gomp4.Vmhd{ // <vmhd/>
			FullBox: gomp4.FullBox{
				Flags: [3]byte{0, 0, 1},
			},
		})
		if err != nil {
			return err
		}

	case *gortsplib.TrackMPEG4Audio:
		_, err = w.WriteBox(&gomp4.Smhd{ // <smhd/>
		})
		if err != nil {
			return err
		}
	}

	_, err = w.writeBoxStart(&gomp4.Dinf{}) // <dinf>
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(&gomp4.Dref{ // <dref>
		EntryCount: 1,
	})
	if err != nil {
		return err
	}

	_, err = w.WriteBox(&gomp4.Url{ // <url/>
		FullBox: gomp4.FullBox{
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

	_, err = w.writeBoxStart(&gomp4.Stbl{}) // <stbl>
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(&gomp4.Stsd{ // <stsd>
		EntryCount: 1,
	})
	if err != nil {
		return err
	}

	switch ttrack := track.Track.(type) {
	case *gortsplib.TrackH264:
		_, err = w.writeBoxStart(&gomp4.VisualSampleEntry{ // <avc1>
			SampleEntry: gomp4.SampleEntry{
				AnyTypeBox: gomp4.AnyTypeBox{
					Type: gomp4.BoxTypeAvc1(),
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

		_, err = w.WriteBox(&gomp4.AVCDecoderConfiguration{ // <avcc/>
			AnyTypeBox: gomp4.AnyTypeBox{
				Type: gomp4.BoxTypeAvcC(),
			},
			ConfigurationVersion:       1,
			Profile:                    spsp.ProfileIdc,
			ProfileCompatibility:       sps[2],
			Level:                      spsp.LevelIdc,
			LengthSizeMinusOne:         3,
			NumOfSequenceParameterSets: 1,
			SequenceParameterSets: []gomp4.AVCParameterSet{
				{
					Length:  uint16(len(sps)),
					NALUnit: sps,
				},
			},
			NumOfPictureParameterSets: 1,
			PictureParameterSets: []gomp4.AVCParameterSet{
				{
					Length:  uint16(len(pps)),
					NALUnit: pps,
				},
			},
		})
		if err != nil {
			return err
		}

		_, err = w.WriteBox(&gomp4.Btrt{ // <btrt/>
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

	case *gortsplib.TrackMPEG4Audio:
		_, err = w.writeBoxStart(&gomp4.AudioSampleEntry{ // <mp4a>
			SampleEntry: gomp4.SampleEntry{
				AnyTypeBox: gomp4.AnyTypeBox{
					Type: gomp4.BoxTypeMp4a(),
				},
				DataReferenceIndex: 1,
			},
			ChannelCount: uint16(ttrack.Config.ChannelCount),
			SampleSize:   16,
			SampleRate:   uint32(ttrack.ClockRate() * 65536),
		})
		if err != nil {
			return err
		}

		enc, _ := ttrack.Config.Marshal()

		decSpecificInfoTagSize := uint8(len(enc))
		decSpecificInfoTag := append(
			[]byte{
				gomp4.DecSpecificInfoTag,
				0x80, 0x80, 0x80, decSpecificInfoTagSize, // size
			},
			enc...,
		)

		esDescrTag := []byte{
			gomp4.ESDescrTag,
			0x80, 0x80, 0x80, 32 + decSpecificInfoTagSize, // size
			0x00,
			byte(track.ID), // ES_ID
			0x00,
		}

		decoderConfigDescrTag := []byte{
			gomp4.DecoderConfigDescrTag,
			0x80, 0x80, 0x80, 18 + decSpecificInfoTagSize, // size
			0x40, // object type indicator (MPEG-4 Audio)
			0x15, 0x00,
			0x00, 0x00, 0x00, 0x01,
			0xf7, 0x39, 0x00, 0x01,
			0xf7, 0x39,
		}

		slConfigDescrTag := []byte{
			gomp4.SLConfigDescrTag,
			0x80, 0x80, 0x80, 0x01, // size (1)
			0x02,
		}

		data := make([]byte, len(esDescrTag)+len(decoderConfigDescrTag)+len(decSpecificInfoTag)+len(slConfigDescrTag))
		pos := 0

		pos += copy(data[pos:], esDescrTag)
		pos += copy(data[pos:], decoderConfigDescrTag)
		pos += copy(data[pos:], decSpecificInfoTag)
		copy(data[pos:], slConfigDescrTag)

		_, err = w.WriteBox(&myEsds{ // <esds/>
			Data: data,
		})
		if err != nil {
			return err
		}

		_, err = w.WriteBox(&gomp4.Btrt{ // <btrt/>
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
	}

	err = w.writeBoxEnd() // </stsd>
	if err != nil {
		return err
	}

	_, err = w.WriteBox(&gomp4.Stts{ // <stts>
	})
	if err != nil {
		return err
	}

	_, err = w.WriteBox(&gomp4.Stsc{ // <stsc>
	})
	if err != nil {
		return err
	}

	_, err = w.WriteBox(&gomp4.Stsz{ // <stsz>
	})
	if err != nil {
		return err
	}

	_, err = w.WriteBox(&gomp4.Stco{ // <stco>
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
