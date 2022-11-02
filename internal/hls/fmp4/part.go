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

	sampleFlagIsNonSyncSample = 1 << 16
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
		waitingTfdtTfhdTrun
	)

	state := waitingMoof
	var curPart *Part
	var moofOffset uint64
	var curTrack *PartTrack
	var tfdt *gomp4.Tfdt
	var tfhd *gomp4.Tfhd

	_, err := gomp4.ReadBoxStructure(bytes.NewReader(byts), func(h *gomp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type.String() {
		case "moof":
			if state != waitingMoof {
				return nil, fmt.Errorf("unexpected moof")
			}

			curPart = &Part{}
			*ps = append(*ps, curPart)
			moofOffset = h.BoxInfo.Offset
			state = waitingTraf

		case "traf":
			if state != waitingTraf && state != waitingTfdtTfhdTrun {
				return nil, fmt.Errorf("unexpected traf")
			}

			if curTrack != nil {
				if tfdt == nil || tfhd == nil || curTrack.Samples == nil {
					return nil, fmt.Errorf("parse error")
				}
			}

			curTrack = &PartTrack{}
			curPart.Tracks = append(curPart.Tracks, curTrack)
			tfdt = nil
			tfhd = nil
			state = waitingTfdtTfhdTrun

		case "tfhd":
			if state != waitingTfdtTfhdTrun || tfhd != nil {
				return nil, fmt.Errorf("unexpected tfhd")
			}

			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}

			tfhd = box.(*gomp4.Tfhd)
			curTrack.ID = int(tfhd.TrackID)

		case "tfdt":
			if state != waitingTfdtTfhdTrun || tfdt != nil {
				return nil, fmt.Errorf("unexpected tfdt")
			}

			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}

			tfdt = box.(*gomp4.Tfdt)

			if tfdt.FullBox.Version != 1 {
				return nil, fmt.Errorf("unsupported tfdt version")
			}

			curTrack.BaseTime = tfdt.BaseMediaDecodeTimeV1

		case "trun":
			if state != waitingTfdtTfhdTrun || tfhd == nil {
				return nil, fmt.Errorf("unexpected trun")
			}

			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			trun := box.(*gomp4.Trun)

			trunFlags := uint16(trun.Flags[1])<<8 | uint16(trun.Flags[2])
			if (trunFlags & trunFlagDataOffsetPreset) == 0 {
				return nil, fmt.Errorf("unsupported flags")
			}

			existing := len(curTrack.Samples)
			tmp := make([]*PartSample, existing+len(trun.Entries))
			copy(tmp, curTrack.Samples)
			curTrack.Samples = tmp

			ptr := byts[uint64(trun.DataOffset)+moofOffset:]

			for i, e := range trun.Entries {
				s := &PartSample{}

				if (trunFlags & trunFlagSampleDurationPresent) != 0 {
					s.Duration = e.SampleDuration
				} else {
					s.Duration = tfhd.DefaultSampleDuration
				}

				s.PTSOffset = e.SampleCompositionTimeOffsetV1

				var sampleFlags uint32
				if (trunFlags & trunFlagSampleFlagsPresent) != 0 {
					sampleFlags = e.SampleFlags
				} else {
					sampleFlags = tfhd.DefaultSampleFlags
				}
				s.IsNonSyncSample = ((sampleFlags & sampleFlagIsNonSyncSample) != 0)

				var size uint32
				if (trunFlags & trunFlagSampleSizePresent) != 0 {
					size = e.SampleSize
				} else {
					size = tfhd.DefaultSampleSize
				}

				s.Payload = ptr[:size]
				ptr = ptr[size:]

				curTrack.Samples[existing+i] = s
			}

		case "mdat":
			if state != waitingTraf && state != waitingTfdtTfhdTrun {
				return nil, fmt.Errorf("unexpected mdat")
			}

			if curTrack != nil {
				if tfdt == nil || tfhd == nil || curTrack.Samples == nil {
					return nil, fmt.Errorf("parse error")
				}
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
