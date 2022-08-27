package fmp4

import (
	"math"
	"time"

	gomp4 "github.com/abema/go-mp4"
	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
)

func durationGoToMp4(v time.Duration, timescale time.Duration) int64 {
	return int64(math.Round(float64(v*timescale) / float64(time.Second)))
}

func generatePartVideoTraf(
	w *mp4Writer,
	trackID int,
	videoSamples []*VideoSample,
) (*gomp4.Trun, int, error) {
	/*
		traf
		- tfhd
		- tfdt
		- trun
	*/

	_, err := w.WriteBoxStart(&gomp4.Traf{}) // <traf>
	if err != nil {
		return nil, 0, err
	}

	flags := 0

	_, err = w.WriteBox(&gomp4.Tfhd{ // <tfhd/>
		FullBox: gomp4.FullBox{
			Flags: [3]byte{2, byte(flags >> 8), byte(flags)},
		},
		TrackID: uint32(trackID),
	})
	if err != nil {
		return nil, 0, err
	}

	_, err = w.WriteBox(&gomp4.Tfdt{ // <tfdt/>
		FullBox: gomp4.FullBox{
			Version: 1,
		},
		// sum of decode durations of all earlier samples
		BaseMediaDecodeTimeV1: uint64(durationGoToMp4(videoSamples[0].DTS, videoTimescale)),
	})
	if err != nil {
		return nil, 0, err
	}

	flags = 0
	flags |= 0x01  // data offset present
	flags |= 0x100 // sample duration present
	flags |= 0x200 // sample size present
	flags |= 0x400 // sample flags present
	flags |= 0x800 // sample composition time offset present or v1

	trun := &gomp4.Trun{ // <trun/>
		FullBox: gomp4.FullBox{
			Version: 1,
			Flags:   [3]byte{0, byte(flags >> 8), byte(flags)},
		},
		SampleCount: uint32(len(videoSamples)),
	}

	for _, e := range videoSamples {
		off := e.PTS - e.DTS

		flags := uint32(0)
		if !e.IDRPresent {
			flags |= 1 << 16 // sample_is_non_sync_sample
		}

		trun.Entries = append(trun.Entries, gomp4.TrunEntry{
			SampleDuration:                uint32(durationGoToMp4(e.Duration(), videoTimescale)),
			SampleSize:                    uint32(len(e.avcc)),
			SampleFlags:                   flags,
			SampleCompositionTimeOffsetV1: int32(durationGoToMp4(off, videoTimescale)),
		})
	}

	trunOffset, err := w.WriteBox(trun)
	if err != nil {
		return nil, 0, err
	}

	err = w.WriteBoxEnd() // </traf>
	if err != nil {
		return nil, 0, err
	}

	return trun, trunOffset, nil
}

func generatePartAudioTraf(
	w *mp4Writer,
	trackID int,
	audioTrack *gortsplib.TrackMPEG4Audio,
	audioSamples []*AudioSample,
) (*gomp4.Trun, int, error) {
	/*
		traf
		- tfhd
		- tfdt
		- trun
	*/

	if len(audioSamples) == 0 {
		return nil, 0, nil
	}

	_, err := w.WriteBoxStart(&gomp4.Traf{}) // <traf>
	if err != nil {
		return nil, 0, err
	}

	flags := 0

	_, err = w.WriteBox(&gomp4.Tfhd{ // <tfhd/>
		FullBox: gomp4.FullBox{
			Flags: [3]byte{2, byte(flags >> 8), byte(flags)},
		},
		TrackID: uint32(trackID),
	})
	if err != nil {
		return nil, 0, err
	}

	_, err = w.WriteBox(&gomp4.Tfdt{ // <tfdt/>
		FullBox: gomp4.FullBox{
			Version: 1,
		},
		// sum of decode durations of all earlier samples
		BaseMediaDecodeTimeV1: uint64(durationGoToMp4(audioSamples[0].PTS, time.Duration(audioTrack.ClockRate()))),
	})
	if err != nil {
		return nil, 0, err
	}

	flags = 0
	flags |= 0x01  // data offset present
	flags |= 0x100 // sample duration present
	flags |= 0x200 // sample size present

	trun := &gomp4.Trun{ // <trun/>
		FullBox: gomp4.FullBox{
			Version: 0,
			Flags:   [3]byte{0, byte(flags >> 8), byte(flags)},
		},
		SampleCount: uint32(len(audioSamples)),
	}

	for _, e := range audioSamples {
		trun.Entries = append(trun.Entries, gomp4.TrunEntry{
			SampleDuration: uint32(durationGoToMp4(e.Duration(), time.Duration(audioTrack.ClockRate()))),
			SampleSize:     uint32(len(e.AU)),
		})
	}

	trunOffset, err := w.WriteBox(trun)
	if err != nil {
		return nil, 0, err
	}

	err = w.WriteBoxEnd() // </traf>
	if err != nil {
		return nil, 0, err
	}

	return trun, trunOffset, nil
}

// GeneratePart generates a FMP4 part file.
func GeneratePart(
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackMPEG4Audio,
	videoSamples []*VideoSample,
	audioSamples []*AudioSample,
) ([]byte, error) {
	/*
		moof
		- mfhd
		- traf (video)
		- traf (audio)
		mdat
	*/

	w := newMP4Writer()

	moofOffset, err := w.WriteBoxStart(&gomp4.Moof{}) // <moof>
	if err != nil {
		return nil, err
	}

	_, err = w.WriteBox(&gomp4.Mfhd{ // <mfhd/>
		SequenceNumber: 0,
	})
	if err != nil {
		return nil, err
	}

	trackID := 1

	var videoTrun *gomp4.Trun
	var videoTrunOffset int
	if videoTrack != nil {
		for _, e := range videoSamples {
			var err error
			e.avcc, err = h264.AVCCMarshal(e.NALUs)
			if err != nil {
				return nil, err
			}
		}

		var err error
		videoTrun, videoTrunOffset, err = generatePartVideoTraf(
			w, trackID, videoSamples)
		if err != nil {
			return nil, err
		}

		trackID++
	}

	var audioTrun *gomp4.Trun
	var audioTrunOffset int
	if audioTrack != nil {
		var err error
		audioTrun, audioTrunOffset, err = generatePartAudioTraf(w, trackID, audioTrack, audioSamples)
		if err != nil {
			return nil, err
		}
	}

	err = w.WriteBoxEnd() // </moof>
	if err != nil {
		return nil, err
	}

	mdat := &gomp4.Mdat{} // <mdat/>

	dataSize := 0
	videoDataSize := 0

	if videoTrack != nil {
		for _, e := range videoSamples {
			dataSize += len(e.avcc)
		}
		videoDataSize = dataSize
	}

	if audioTrack != nil {
		for _, e := range audioSamples {
			dataSize += len(e.AU)
		}
	}

	mdat.Data = make([]byte, dataSize)
	pos := 0

	if videoTrack != nil {
		for _, e := range videoSamples {
			pos += copy(mdat.Data[pos:], e.avcc)
		}
	}

	if audioTrack != nil {
		for _, e := range audioSamples {
			pos += copy(mdat.Data[pos:], e.AU)
		}
	}

	mdatOffset, err := w.WriteBox(mdat)
	if err != nil {
		return nil, err
	}

	if videoTrack != nil {
		videoTrun.DataOffset = int32(mdatOffset - moofOffset + 8)
		err = w.RewriteBox(videoTrunOffset, videoTrun)
		if err != nil {
			return nil, err
		}
	}

	if audioTrack != nil && audioTrun != nil {
		audioTrun.DataOffset = int32(videoDataSize + mdatOffset - moofOffset + 8)
		err = w.RewriteBox(audioTrunOffset, audioTrun)
		if err != nil {
			return nil, err
		}
	}

	return w.Bytes(), nil
}
