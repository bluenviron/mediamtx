package fmp4

import (
	"bytes"
	"fmt"

	gomp4 "github.com/abema/go-mp4"
	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/mpeg4audio"
)

// Init is a FMP4 initialization file.
type Init struct {
	VideoTrack *InitTrack
	AudioTrack *InitTrack
}

// Unmarshal decodes a FMP4 initialization file.
func (i *Init) Unmarshal(byts []byte) error {
	type readState int

	const (
		waitingTrak readState = iota
		waitingTkhd
		waitingMdhd
		waitingCodec
		waitingAvcc
		waitingEsds
	)

	state := waitingTrak
	var curTrack *InitTrack

	_, err := gomp4.ReadBoxStructure(bytes.NewReader(byts), func(h *gomp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type.String() {
		case "trak":
			if state != waitingTrak {
				return nil, fmt.Errorf("parse error")
			}

			curTrack = &InitTrack{}
			state = waitingTkhd

		case "tkhd":
			if state != waitingTkhd {
				return nil, fmt.Errorf("parse error")
			}

			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			tkhd := box.(*gomp4.Tkhd)

			curTrack.ID = int(tkhd.TrackID)
			state = waitingMdhd

		case "mdhd":
			if state != waitingMdhd {
				return nil, fmt.Errorf("parse error")
			}

			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			mdhd := box.(*gomp4.Mdhd)

			curTrack.TimeScale = mdhd.Timescale
			state = waitingCodec

		case "avc1":
			if state != waitingCodec {
				return nil, fmt.Errorf("parse error")
			}

			if i.VideoTrack != nil {
				return nil, fmt.Errorf("multiple video tracks are not supported")
			}

			state = waitingAvcc

		case "avcC":
			if state != waitingAvcc {
				return nil, fmt.Errorf("parse error")
			}

			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			conf := box.(*gomp4.AVCDecoderConfiguration)

			if len(conf.SequenceParameterSets) > 1 {
				return nil, fmt.Errorf("multiple SPS are not supported")
			}

			var sps []byte
			if len(conf.SequenceParameterSets) == 1 {
				sps = conf.SequenceParameterSets[0].NALUnit
			}

			if len(conf.PictureParameterSets) > 1 {
				return nil, fmt.Errorf("multiple PPS are not supported")
			}

			var pps []byte
			if len(conf.PictureParameterSets) == 1 {
				pps = conf.PictureParameterSets[0].NALUnit
			}

			curTrack.Track = &gortsplib.TrackH264{
				PayloadType: 96,
				SPS:         sps,
				PPS:         pps,
			}
			i.VideoTrack = curTrack
			state = waitingTrak

		case "mp4a":
			if state != waitingCodec {
				return nil, fmt.Errorf("parse error")
			}

			if i.AudioTrack != nil {
				return nil, fmt.Errorf("multiple audio tracks are not supported")
			}

			state = waitingEsds

		case "esds":
			if state != waitingEsds {
				return nil, fmt.Errorf("parse error")
			}

			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			esds := box.(*gomp4.Esds)

			encodedConf := func() []byte {
				for _, desc := range esds.Descriptors {
					if desc.Tag == gomp4.DecSpecificInfoTag {
						return desc.Data
					}
				}
				return nil
			}()
			if encodedConf == nil {
				return nil, fmt.Errorf("unable to find MPEG4-audio configuration")
			}

			var c mpeg4audio.Config
			err = c.Unmarshal(encodedConf)
			if err != nil {
				return nil, fmt.Errorf("invalid MPEG4-audio configuration: %s", err)
			}

			curTrack.Track = &gortsplib.TrackMPEG4Audio{
				PayloadType:      96,
				Config:           &c,
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			}
			i.AudioTrack = curTrack
			state = waitingTrak
		}

		return h.Expand()
	})
	if err != nil {
		return err
	}

	if state != waitingTrak {
		return fmt.Errorf("parse error")
	}

	if i.VideoTrack == nil && i.AudioTrack == nil {
		return fmt.Errorf("no tracks found")
	}

	return nil
}

// Marshal encodes a FMP4 initialization file.
func (i *Init) Marshal() ([]byte, error) {
	/*
		- ftyp
		- moov
		  - mvhd
		  - trak
		  - trak
		  - ...
		- mvex
		  - trex
		  - trex
		  - ...
	*/

	w := newMP4Writer()

	_, err := w.WriteBox(&gomp4.Ftyp{ // <ftyp/>
		MajorBrand:   [4]byte{'m', 'p', '4', '2'},
		MinorVersion: 1,
		CompatibleBrands: []gomp4.CompatibleBrandElem{
			{CompatibleBrand: [4]byte{'m', 'p', '4', '1'}},
			{CompatibleBrand: [4]byte{'m', 'p', '4', '2'}},
			{CompatibleBrand: [4]byte{'i', 's', 'o', 'm'}},
			{CompatibleBrand: [4]byte{'h', 'l', 's', 'f'}},
		},
	})
	if err != nil {
		return nil, err
	}

	_, err = w.writeBoxStart(&gomp4.Moov{}) // <moov>
	if err != nil {
		return nil, err
	}

	_, err = w.WriteBox(&gomp4.Mvhd{ // <mvhd/>
		Timescale:   1000,
		Rate:        65536,
		Volume:      256,
		Matrix:      [9]int32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
		NextTrackID: 4294967295,
	})
	if err != nil {
		return nil, err
	}

	if i.VideoTrack != nil {
		err := i.VideoTrack.marshal(w)
		if err != nil {
			return nil, err
		}
	}

	if i.AudioTrack != nil {
		err := i.AudioTrack.marshal(w)
		if err != nil {
			return nil, err
		}
	}

	_, err = w.writeBoxStart(&gomp4.Mvex{}) // <mvex>
	if err != nil {
		return nil, err
	}

	if i.VideoTrack != nil {
		_, err = w.WriteBox(&gomp4.Trex{ // <trex/>
			TrackID:                       uint32(i.VideoTrack.ID),
			DefaultSampleDescriptionIndex: 1,
		})
		if err != nil {
			return nil, err
		}
	}

	if i.AudioTrack != nil {
		_, err = w.WriteBox(&gomp4.Trex{ // <trex/>
			TrackID:                       uint32(i.AudioTrack.ID),
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
