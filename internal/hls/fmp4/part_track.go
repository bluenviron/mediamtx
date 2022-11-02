package fmp4

import (
	gomp4 "github.com/abema/go-mp4"
)

// PartSample is a sample of a PartTrack.
type PartSample struct {
	Duration        uint32
	PTSOffset       int32
	IsNonSyncSample bool
	Payload         []byte
}

// PartTrack is a track of Part.
type PartTrack struct {
	ID       int
	BaseTime uint64
	Samples  []*PartSample
	IsVideo  bool // marshal only
}

func (pt *PartTrack) marshal(w *mp4Writer) (*gomp4.Trun, int, error) {
	/*
		traf
		- tfhd
		- tfdt
		- trun
	*/

	_, err := w.writeBoxStart(&gomp4.Traf{}) // <traf>
	if err != nil {
		return nil, 0, err
	}

	flags := 0

	_, err = w.WriteBox(&gomp4.Tfhd{ // <tfhd/>
		FullBox: gomp4.FullBox{
			Flags: [3]byte{2, byte(flags >> 8), byte(flags)},
		},
		TrackID: uint32(pt.ID),
	})
	if err != nil {
		return nil, 0, err
	}

	_, err = w.WriteBox(&gomp4.Tfdt{ // <tfdt/>
		FullBox: gomp4.FullBox{
			Version: 1,
		},
		// sum of decode durations of all earlier samples
		BaseMediaDecodeTimeV1: pt.BaseTime,
	})
	if err != nil {
		return nil, 0, err
	}

	if pt.IsVideo {
		flags = trunFlagDataOffsetPreset |
			trunFlagSampleDurationPresent |
			trunFlagSampleSizePresent |
			trunFlagSampleFlagsPresent |
			trunFlagSampleCompositionTimeOffsetPresentOrV1
	} else {
		flags = trunFlagDataOffsetPreset |
			trunFlagSampleDurationPresent |
			trunFlagSampleSizePresent
	}

	trun := &gomp4.Trun{ // <trun/>
		FullBox: gomp4.FullBox{
			Version: 1,
			Flags:   [3]byte{0, byte(flags >> 8), byte(flags)},
		},
		SampleCount: uint32(len(pt.Samples)),
	}

	for _, sample := range pt.Samples {
		if pt.IsVideo {
			var flags uint32
			if sample.IsNonSyncSample {
				flags |= sampleFlagIsNonSyncSample
			}

			trun.Entries = append(trun.Entries, gomp4.TrunEntry{
				SampleDuration:                sample.Duration,
				SampleSize:                    uint32(len(sample.Payload)),
				SampleFlags:                   flags,
				SampleCompositionTimeOffsetV1: sample.PTSOffset,
			})
		} else {
			trun.Entries = append(trun.Entries, gomp4.TrunEntry{
				SampleDuration: sample.Duration,
				SampleSize:     uint32(len(sample.Payload)),
			})
		}
	}

	trunOffset, err := w.WriteBox(trun)
	if err != nil {
		return nil, 0, err
	}

	err = w.writeBoxEnd() // </traf>
	if err != nil {
		return nil, 0, err
	}

	return trun, trunOffset, nil
}
