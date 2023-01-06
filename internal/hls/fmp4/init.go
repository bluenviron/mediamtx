package fmp4

import (
	"bytes"
	"fmt"

	gomp4 "github.com/abema/go-mp4"
	"github.com/aler9/gortsplib/v2/pkg/codecs/h265"
	"github.com/aler9/gortsplib/v2/pkg/codecs/mpeg4audio"
	"github.com/aler9/gortsplib/v2/pkg/format"
)

// Init is a FMP4 initialization file.
type Init struct {
	Tracks []*InitTrack
}

// Unmarshal decodes a FMP4 initialization file.
func (i *Init) Unmarshal(byts []byte) error {
	type readState int

	const (
		waitingTrak readState = iota
		waitingTkhd
		waitingMdhd
		waitingCodec
		waitingAvcC
		waitingHvcC
		waitingEsds
		waitingDOps
	)

	state := waitingTrak
	var curTrack *InitTrack

	_, err := gomp4.ReadBoxStructure(bytes.NewReader(byts), func(h *gomp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type.String() {
		case "trak":
			if state != waitingTrak {
				return nil, fmt.Errorf("unexpected box 'trak'")
			}

			curTrack = &InitTrack{}
			i.Tracks = append(i.Tracks, curTrack)
			state = waitingTkhd

		case "tkhd":
			if state != waitingTkhd {
				return nil, fmt.Errorf("unexpected box 'tkhd'")
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
				return nil, fmt.Errorf("unexpected box 'mdhd'")
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
				return nil, fmt.Errorf("unexpected box 'avc1'")
			}
			state = waitingAvcC

		case "avcC":
			if state != waitingAvcC {
				return nil, fmt.Errorf("unexpected box 'avcC'")
			}

			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			avcc := box.(*gomp4.AVCDecoderConfiguration)

			if len(avcc.SequenceParameterSets) > 1 {
				return nil, fmt.Errorf("multiple SPS are not supported")
			}

			var sps []byte
			if len(avcc.SequenceParameterSets) == 1 {
				sps = avcc.SequenceParameterSets[0].NALUnit
			}

			if len(avcc.PictureParameterSets) > 1 {
				return nil, fmt.Errorf("multiple PPS are not supported")
			}

			var pps []byte
			if len(avcc.PictureParameterSets) == 1 {
				pps = avcc.PictureParameterSets[0].NALUnit
			}

			curTrack.Format = &format.H264{
				PayloadTyp:        96,
				SPS:               sps,
				PPS:               pps,
				PacketizationMode: 1,
			}
			state = waitingTrak

		case "hev1", "hvc1":
			if state != waitingCodec {
				return nil, fmt.Errorf("unexpected box 'hev1'")
			}
			state = waitingHvcC

		case "hvcC":
			if state != waitingHvcC {
				return nil, fmt.Errorf("unexpected box 'hvcC'")
			}

			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			hvcc := box.(*gomp4.HvcC)

			var vps []byte
			var sps []byte
			var pps []byte

			for _, arr := range hvcc.NaluArrays {
				switch h265.NALUType(arr.NaluType) {
				case h265.NALUType_VPS_NUT, h265.NALUType_SPS_NUT, h265.NALUType_PPS_NUT:
					if arr.NumNalus != 1 {
						return nil, fmt.Errorf("multiple VPS/SPS/PPS are not supported")
					}
				}

				switch h265.NALUType(arr.NaluType) {
				case h265.NALUType_VPS_NUT:
					vps = arr.Nalus[0].NALUnit

				case h265.NALUType_SPS_NUT:
					sps = arr.Nalus[0].NALUnit

				case h265.NALUType_PPS_NUT:
					pps = arr.Nalus[0].NALUnit
				}
			}

			if vps == nil {
				return nil, fmt.Errorf("VPS not provided")
			}

			if sps == nil {
				return nil, fmt.Errorf("SPS not provided")
			}

			if pps == nil {
				return nil, fmt.Errorf("PPS not provided")
			}

			curTrack.Format = &format.H265{
				PayloadTyp: 96,
				VPS:        vps,
				SPS:        sps,
				PPS:        pps,
			}
			state = waitingTrak

		case "mp4a":
			if state != waitingCodec {
				return nil, fmt.Errorf("unexpected box 'mp4a'")
			}
			state = waitingEsds

		case "esds":
			if state != waitingEsds {
				return nil, fmt.Errorf("unexpected box 'esds'")
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

			curTrack.Format = &format.MPEG4Audio{
				PayloadTyp:       96,
				Config:           &c,
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			}
			state = waitingTrak

		case "Opus":
			if state != waitingCodec {
				return nil, fmt.Errorf("unexpected box 'Opus'")
			}
			state = waitingDOps

		case "dOps":
			if state != waitingDOps {
				return nil, fmt.Errorf("unexpected box 'dOps'")
			}

			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			dops := box.(*DOps)

			curTrack.Format = &format.Opus{
				PayloadTyp:   96,
				SampleRate:   int(dops.InputSampleRate),
				ChannelCount: int(dops.OutputChannelCount),
			}
			state = waitingTrak

		case "ac-3": // ac-3, not supported yet
			i.Tracks = i.Tracks[:len(i.Tracks)-1]
			state = waitingTrak
			return nil, nil

		case "ec-3": // ec-3, not supported yet
			i.Tracks = i.Tracks[:len(i.Tracks)-1]
			state = waitingTrak
			return nil, nil

		case "c608", "c708": // closed captions, not supported yet
			i.Tracks = i.Tracks[:len(i.Tracks)-1]
			state = waitingTrak
			return nil, nil

		case "chrm", "nmhd":
			return nil, nil
		}

		return h.Expand()
	})
	if err != nil {
		return err
	}

	if state != waitingTrak {
		return fmt.Errorf("parse error")
	}

	if len(i.Tracks) == 0 {
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

	for _, track := range i.Tracks {
		err := track.marshal(w)
		if err != nil {
			return nil, err
		}
	}

	_, err = w.writeBoxStart(&gomp4.Mvex{}) // <mvex>
	if err != nil {
		return nil, err
	}

	for _, track := range i.Tracks {
		_, err = w.WriteBox(&gomp4.Trex{ // <trex/>
			TrackID:                       uint32(track.ID),
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
