package fmp4

import (
	"bytes"
	"fmt"

	gomp4 "github.com/abema/go-mp4"
)

type partReadState int

const (
	waitingTraf partReadState = iota
	waitingTfhd
	waitingTfdt
	waitingTrun
)

// PartRead reads a FMP4 part file.
func PartRead(
	byts []byte,
	cb func(),
) error {
	state := waitingTraf
	var trackID uint32
	var baseTime uint64
	var entries []gomp4.TrunEntry

	_, err := gomp4.ReadBoxStructure(bytes.NewReader(byts), func(h *gomp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type.String() {
		case "traf":
			if state != waitingTraf {
				return nil, fmt.Errorf("decode error")
			}
			state = waitingTfhd

		case "tfhd":
			if state != waitingTfhd {
				return nil, fmt.Errorf("decode error")
			}

			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			trackID = box.(*gomp4.Tfhd).TrackID

			state = waitingTfdt

		case "tfdt":
			if state != waitingTfdt {
				return nil, fmt.Errorf("decode error")
			}

			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			t := box.(*gomp4.Tfdt)

			if t.FullBox.Version != 1 {
				return nil, fmt.Errorf("unsupported tfdt version")
			}

			baseTime = t.BaseMediaDecodeTimeV1
			state = waitingTrun

		case "trun":
			if state != waitingTrun {
				return nil, fmt.Errorf("decode error")
			}

			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			t := box.(*gomp4.Trun)

			entries = t.Entries
			state = waitingTraf
		}

		return h.Expand()
	})
	if err != nil {
		return err
	}

	if state != waitingTraf {
		return fmt.Errorf("parse error")
	}

	fmt.Println("TODO", trackID, baseTime, entries)

	return nil
}
