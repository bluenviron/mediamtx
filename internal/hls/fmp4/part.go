package fmp4

import (
	"bytes"
	"fmt"

	gomp4 "github.com/abema/go-mp4"
)

const (
	trunFlagDataOffsetPreset                       = 0x01
	trunFlagSampleDurationPresent                  = 0x100
	trunFlagSampleSizePresent                      = 0x200
	trunFlagSampleFlagsPresent                     = 0x400
	trunFlagSampleCompositionTimeOffsetPresentOrV1 = 0x800
)

// Part is a FMP4 part file.
type Part struct {
	Tracks []*PartTrack
}

// Parts is a sequence of FMP4 parts.
type Parts []*Part

// Unmarshal decodes one or more FMP4 parts.
func (ps *Parts) Unmarshal(byts []byte) error {
	type readState int

	const (
		waitingMoof readState = iota
		waitingTraf
		waitingTfhd
		waitingTfdt
		waitingTrun
	)

	state := waitingMoof
	var curPart *Part
	var moofOffset uint64
	var curTrack *PartTrack
	var defaultSampleDuration uint32
	var defaultSampleFlags uint32
	var defaultSampleSize uint32

	_, err := gomp4.ReadBoxStructure(bytes.NewReader(byts), func(h *gomp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type.String() {
		case "moof":
			if state != waitingMoof {
				return nil, fmt.Errorf("decode error")
			}

			curPart = &Part{}
			*ps = append(*ps, curPart)
			moofOffset = h.BoxInfo.Offset
			state = waitingTraf

		case "traf":
			if state != waitingTraf {
				return nil, fmt.Errorf("decode error")
			}

			curTrack = &PartTrack{}
			curPart.Tracks = append(curPart.Tracks, curTrack)
			state = waitingTfhd

		case "tfhd":
			if state != waitingTfhd {
				return nil, fmt.Errorf("decode error")
			}

			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			tfhd := box.(*gomp4.Tfhd)

			curTrack.ID = int(tfhd.TrackID)

			defaultSampleDuration = tfhd.DefaultSampleDuration
			defaultSampleFlags = tfhd.DefaultSampleFlags
			defaultSampleSize = tfhd.DefaultSampleSize
			state = waitingTfdt

		case "tfdt":
			if state != waitingTfdt {
				return nil, fmt.Errorf("decode error")
			}

			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			tfdt := box.(*gomp4.Tfdt)

			if tfdt.FullBox.Version != 1 {
				return nil, fmt.Errorf("unsupported tfdt version")
			}

			curTrack.BaseTime = tfdt.BaseMediaDecodeTimeV1
			state = waitingTrun

		case "trun":
			if state != waitingTrun {
				return nil, fmt.Errorf("decode error")
			}

			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			trun := box.(*gomp4.Trun)

			flags := uint16(trun.Flags[1])<<8 | uint16(trun.Flags[2])
			if (flags & trunFlagDataOffsetPreset) == 0 {
				return nil, fmt.Errorf("unsupported flags")
			}

			curTrack.Samples = make([]*PartSample, len(trun.Entries))
			ptr := byts[uint64(trun.DataOffset)+moofOffset:]

			for i, e := range trun.Entries {
				s := &PartSample{}

				if (flags & trunFlagSampleDurationPresent) != 0 {
					s.Duration = e.SampleDuration
				} else {
					s.Duration = defaultSampleDuration
				}

				s.PTSOffset = e.SampleCompositionTimeOffsetV1

				if (flags & trunFlagSampleFlagsPresent) != 0 {
					s.Flags = e.SampleFlags
				} else {
					s.Flags = defaultSampleFlags
				}

				var size uint32
				if (flags & trunFlagSampleSizePresent) != 0 {
					size = e.SampleSize
				} else {
					size = defaultSampleSize
				}

				s.Payload = ptr[:size]
				ptr = ptr[size:]

				curTrack.Samples[i] = s
			}

			state = waitingTraf

		case "mdat":
			if state != waitingTraf {
				return nil, fmt.Errorf("decode error")
			}
			state = waitingMoof
			return nil, nil
		}

		return h.Expand()
	})
	if err != nil {
		return err
	}

	if state != waitingMoof {
		return fmt.Errorf("decode error")
	}

	return nil
}

// Marshal encodes a FMP4 part file.
func (p *Part) Marshal() ([]byte, error) {
	/*
		moof
		- mfhd
		- traf (video)
		- traf (audio)
		mdat
	*/

	w := newMP4Writer()

	moofOffset, err := w.writeBoxStart(&gomp4.Moof{}) // <moof>
	if err != nil {
		return nil, err
	}

	_, err = w.WriteBox(&gomp4.Mfhd{ // <mfhd/>
		SequenceNumber: 0,
	})
	if err != nil {
		return nil, err
	}

	trackLen := len(p.Tracks)
	truns := make([]*gomp4.Trun, trackLen)
	trunOffsets := make([]int, trackLen)
	dataOffsets := make([]int, trackLen)
	dataSize := 0

	for i, track := range p.Tracks {
		trun, trunOffset, err := track.marshal(w)
		if err != nil {
			return nil, err
		}

		dataOffsets[i] = dataSize

		for _, sample := range track.Samples {
			dataSize += len(sample.Payload)
		}

		truns[i] = trun
		trunOffsets[i] = trunOffset
	}

	err = w.writeBoxEnd() // </moof>
	if err != nil {
		return nil, err
	}

	mdat := &gomp4.Mdat{} // <mdat/>
	mdat.Data = make([]byte, dataSize)
	pos := 0

	for _, track := range p.Tracks {
		for _, sample := range track.Samples {
			pos += copy(mdat.Data[pos:], sample.Payload)
		}
	}

	mdatOffset, err := w.WriteBox(mdat)
	if err != nil {
		return nil, err
	}

	for i := range p.Tracks {
		truns[i].DataOffset = int32(dataOffsets[i] + mdatOffset - moofOffset + 8)
		err = w.rewriteBox(trunOffsets[i], truns[i])
		if err != nil {
			return nil, err
		}
	}

	return w.bytes(), nil
}
