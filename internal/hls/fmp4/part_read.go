package fmp4

import (
	"bytes"
	"fmt"

	gomp4 "github.com/abema/go-mp4"
)

// Subpart is a sub-part of a FMP4 part.
// It contains a single track and a series of entries.
type Subpart struct {
	ID       int
	BaseTime uint64
	Entries  []*gomp4.TrunEntry
	Data     []byte
}

// PartRead reads a FMP4 part file.
func PartRead(
	byts []byte,
) ([]*Subpart, error) {
	type readState int

	const (
		waitingMoof readState = iota
		waitingTraf
		waitingTfhd
		waitingTfdt
		waitingTrun
	)

	state := waitingMoof
	var moofOffset uint64
	var curTrack *Subpart
	var tracks []*Subpart
	var defaultSampleDuration uint32
	var defaultSampleFlags uint32
	var defaultSampleSize uint32

	_, err := gomp4.ReadBoxStructure(bytes.NewReader(byts), func(h *gomp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type.String() {
		case "moof":
			if state != waitingMoof {
				return nil, fmt.Errorf("decode error")
			}

			moofOffset = h.BoxInfo.Offset
			state = waitingTraf

		case "traf":
			if state != waitingTraf {
				return nil, fmt.Errorf("decode error")
			}

			curTrack = &Subpart{}
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

			curTrack.Entries = make([]*gomp4.TrunEntry, len(trun.Entries))

			for i := range trun.Entries {
				e := &trun.Entries[i]

				if (flags & trunFlagSampleDurationPresent) == 0 {
					e.SampleDuration = defaultSampleDuration
				}
				if (flags & trunFlagSampleFlagsPresent) == 0 {
					e.SampleFlags = defaultSampleFlags
				}
				if (flags & trunFlagSampleSizePresent) == 0 {
					e.SampleSize = defaultSampleSize
				}

				curTrack.Entries[i] = e
			}

			curTrack.Data = byts[uint64(trun.DataOffset)+moofOffset:]
			tracks = append(tracks, curTrack)
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
		return nil, err
	}

	return tracks, nil
}
