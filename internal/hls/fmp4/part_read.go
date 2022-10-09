package fmp4

import (
	"bytes"
	"fmt"

	gomp4 "github.com/abema/go-mp4"
)

type partReadState int

const (
	waitingMoof partReadState = iota
	waitingTraf
	waitingTfhd
	waitingTfdt
	waitingTrun
)

// PartTrack is a track of a part file.
type PartTrack struct {
	ID       uint32
	BaseTime uint64
	Entries  []gomp4.TrunEntry
	Data     []byte
}

// PartRead reads a FMP4 part file.
func PartRead(
	byts []byte,
) ([]*PartTrack, error) {
	state := waitingMoof
	var moofOffset uint64
	var curTrack *PartTrack
	var tracks []*PartTrack

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

			curTrack = &PartTrack{}
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

			curTrack.ID = tfhd.TrackID
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
			if (flags & 0x100) == 0 { // sample duration present
				return nil, fmt.Errorf("unsupported flags")
			}
			if (flags & 0x200) == 0 { // sample size present
				return nil, fmt.Errorf("unsupported flags")
			}

			curTrack.Entries = trun.Entries
			o := uint64(trun.DataOffset) + moofOffset
			curTrack.Data = byts[o:]
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
