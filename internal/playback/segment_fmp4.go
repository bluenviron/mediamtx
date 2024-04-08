package playback

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/abema/go-mp4"
)

const (
	sampleFlagIsNonSyncSample = 1 << 16
	concatenationTolerance    = 500 * time.Millisecond
	fmp4Timescale             = 90000
)

func durationGoToMp4(v time.Duration, timeScale uint32) uint64 {
	timeScale64 := uint64(timeScale)
	secs := v / time.Second
	dec := v % time.Second
	return uint64(secs)*timeScale64 + uint64(dec)*timeScale64/uint64(time.Second)
}

func durationMp4ToGo(v uint64, timeScale uint32) time.Duration {
	timeScale64 := uint64(timeScale)
	secs := v / timeScale64
	dec := v % timeScale64
	return time.Duration(secs)*time.Second + time.Duration(dec)*time.Second/time.Duration(timeScale64)
}

var errTerminated = errors.New("terminated")

func segmentFMP4CanBeConcatenated(
	prevInit []byte,
	prevEnd time.Time,
	curInit []byte,
	curStart time.Time,
) bool {
	return bytes.Equal(prevInit, curInit) &&
		!curStart.Before(prevEnd.Add(-concatenationTolerance)) &&
		!curStart.After(prevEnd.Add(concatenationTolerance))
}

func segmentFMP4ReadInit(r io.ReadSeeker) ([]byte, error) {
	buf := make([]byte, 8)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}

	// find ftyp

	if !bytes.Equal(buf[4:], []byte{'f', 't', 'y', 'p'}) {
		return nil, fmt.Errorf("ftyp box not found")
	}

	ftypSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

	_, err = r.Seek(int64(ftypSize), io.SeekStart)
	if err != nil {
		return nil, err
	}

	// find moov

	_, err = io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}

	if !bytes.Equal(buf[4:], []byte{'m', 'o', 'o', 'v'}) {
		return nil, fmt.Errorf("moov box not found")
	}

	moovSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

	_, err = r.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}

	// return ftyp and moov

	buf = make([]byte, ftypSize+moovSize)

	_, err = io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

func segmentFMP4ReadMaxDuration(r io.ReadSeeker) (time.Duration, error) {
	// find and skip ftyp

	buf := make([]byte, 8)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return 0, err
	}

	if !bytes.Equal(buf[4:], []byte{'f', 't', 'y', 'p'}) {
		return 0, fmt.Errorf("ftyp box not found")
	}

	ftypSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

	_, err = r.Seek(int64(ftypSize), io.SeekStart)
	if err != nil {
		return 0, err
	}

	// find and skip moov

	_, err = io.ReadFull(r, buf)
	if err != nil {
		return 0, err
	}

	if !bytes.Equal(buf[4:], []byte{'m', 'o', 'o', 'v'}) {
		return 0, fmt.Errorf("moov box not found")
	}

	moovSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

	_, err = r.Seek(int64(moovSize)-8, io.SeekCurrent)
	if err != nil {
		return 0, err
	}

	// find last valid moof and mdat

	lastMoofPos := int64(-1)

	for {
		moofPos, err := r.Seek(0, io.SeekCurrent)
		if err != nil {
			return 0, err
		}

		_, err = io.ReadFull(r, buf)
		if err != nil {
			break
		}

		if !bytes.Equal(buf[4:], []byte{'m', 'o', 'o', 'f'}) {
			return 0, fmt.Errorf("moof box not found")
		}

		moofSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

		_, err = r.Seek(int64(moofSize)-8, io.SeekCurrent)
		if err != nil {
			break
		}

		_, err = io.ReadFull(r, buf)
		if err != nil {
			break
		}

		if !bytes.Equal(buf[4:], []byte{'m', 'd', 'a', 't'}) {
			return 0, fmt.Errorf("mdat box not found")
		}

		mdatSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

		_, err = r.Seek(int64(mdatSize)-8, io.SeekCurrent)
		if err != nil {
			break
		}

		lastMoofPos = moofPos
	}

	if lastMoofPos < 0 {
		return 0, fmt.Errorf("no moof boxes found")
	}

	// open last moof

	_, err = r.Seek(lastMoofPos+8, io.SeekStart)
	if err != nil {
		return 0, err
	}

	_, err = io.ReadFull(r, buf)
	if err != nil {
		return 0, err
	}

	// skip mfhd

	if !bytes.Equal(buf[4:], []byte{'m', 'f', 'h', 'd'}) {
		return 0, fmt.Errorf("mfhd box not found")
	}

	_, err = r.Seek(8, io.SeekCurrent)
	if err != nil {
		return 0, err
	}

	maxElapsed := uint64(0)

	// foreach traf

	for {
		_, err := io.ReadFull(r, buf)
		if err != nil {
			return 0, err
		}

		if !bytes.Equal(buf[4:], []byte{'t', 'r', 'a', 'f'}) {
			if bytes.Equal(buf[4:], []byte{'m', 'd', 'a', 't'}) {
				break
			}
			return 0, fmt.Errorf("traf box not found")
		}

		// skip tfhd

		_, err = io.ReadFull(r, buf)
		if err != nil {
			return 0, err
		}

		if !bytes.Equal(buf[4:], []byte{'t', 'f', 'h', 'd'}) {
			return 0, fmt.Errorf("tfhd box not found")
		}

		tfhdSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

		_, err = r.Seek(int64(tfhdSize)-8, io.SeekCurrent)
		if err != nil {
			return 0, err
		}

		// parse tfdt

		_, err = io.ReadFull(r, buf)
		if err != nil {
			return 0, err
		}

		if !bytes.Equal(buf[4:], []byte{'t', 'f', 'd', 't'}) {
			return 0, fmt.Errorf("tfdt box not found")
		}

		tfdtSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

		buf2 := make([]byte, tfdtSize-8)

		_, err = io.ReadFull(r, buf2)
		if err != nil {
			return 0, err
		}

		var tfdt mp4.Tfdt
		_, err = mp4.Unmarshal(bytes.NewReader(buf2), uint64(len(buf2)), &tfdt, mp4.Context{})
		if err != nil {
			return 0, fmt.Errorf("invalid tfdt box: %w", err)
		}

		// parse trun

		_, err = io.ReadFull(r, buf)
		if err != nil {
			return 0, err
		}

		if !bytes.Equal(buf[4:], []byte{'t', 'r', 'u', 'n'}) {
			return 0, fmt.Errorf("trun box not found")
		}

		trunSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

		buf2 = make([]byte, trunSize-8)

		_, err = io.ReadFull(r, buf2)
		if err != nil {
			return 0, err
		}

		var trun mp4.Trun
		_, err = mp4.Unmarshal(bytes.NewReader(buf2), uint64(len(buf2)), &trun, mp4.Context{})
		if err != nil {
			return 0, fmt.Errorf("invalid trun box: %w", err)
		}

		elapsed := tfdt.BaseMediaDecodeTimeV1

		for _, entry := range trun.Entries {
			elapsed += uint64(entry.SampleDuration)
		}

		if elapsed > maxElapsed {
			maxElapsed = elapsed
		}
	}

	return durationMp4ToGo(maxElapsed, fmp4Timescale), nil
}

func segmentFMP4SeekAndMuxParts(
	r io.ReadSeeker,
	segmentStartOffset time.Duration,
	duration time.Duration,
	m muxer,
) (time.Duration, error) {
	segmentStartOffsetMP4 := durationGoToMp4(segmentStartOffset, fmp4Timescale)
	durationMP4 := durationGoToMp4(duration, fmp4Timescale)
	moofOffset := uint64(0)
	var tfhd *mp4.Tfhd
	var tfdt *mp4.Tfdt
	atLeastOnePartWritten := false
	maxMuxerDTS := int64(0)
	breakAtNextMdat := false

	_, err := mp4.ReadBoxStructure(r, func(h *mp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type.String() {
		case "moof":
			moofOffset = h.BoxInfo.Offset
			return h.Expand()

		case "traf":
			return h.Expand()

		case "tfhd":
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			tfhd = box.(*mp4.Tfhd)

		case "tfdt":
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			tfdt = box.(*mp4.Tfdt)

			m.setTrack(int(tfhd.TrackID))

		case "trun":
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			trun := box.(*mp4.Trun)

			dataOffset := moofOffset + uint64(trun.DataOffset)

			_, err = r.Seek(int64(dataOffset), io.SeekStart)
			if err != nil {
				return nil, err
			}

			muxerDTS := int64(tfdt.BaseMediaDecodeTimeV1) - int64(segmentStartOffsetMP4)

			for _, e := range trun.Entries {
				if muxerDTS >= int64(durationMP4) {
					breakAtNextMdat = true
					break
				}

				if muxerDTS >= 0 {
					atLeastOnePartWritten = true
				}

				payload := make([]byte, e.SampleSize)
				_, err := io.ReadFull(r, payload)
				if err != nil {
					return nil, err
				}

				err = m.writeSample(
					muxerDTS,
					e.SampleCompositionTimeOffsetV1,
					(e.SampleFlags&sampleFlagIsNonSyncSample) != 0,
					payload,
				)
				if err != nil {
					return nil, err
				}

				muxerDTS += int64(e.SampleDuration)
			}

			m.writeFinalDTS(muxerDTS)

			if muxerDTS > maxMuxerDTS {
				maxMuxerDTS = muxerDTS
			}

		case "mdat":
			if breakAtNextMdat {
				return nil, errTerminated
			}
		}
		return nil, nil
	})
	if err != nil && !errors.Is(err, errTerminated) {
		return 0, err
	}

	if !atLeastOnePartWritten {
		return 0, errNoSegmentsFound
	}

	return durationMp4ToGo(uint64(maxMuxerDTS), fmp4Timescale), nil
}

func segmentFMP4WriteParts(
	r io.ReadSeeker,
	segmentStartOffset time.Duration,
	duration time.Duration,
	m muxer,
) (time.Duration, error) {
	segmentStartOffsetMP4 := durationGoToMp4(segmentStartOffset, fmp4Timescale)
	durationMP4 := durationGoToMp4(duration, fmp4Timescale)
	moofOffset := uint64(0)
	var tfhd *mp4.Tfhd
	var tfdt *mp4.Tfdt
	maxMuxerDTS := int64(0)
	breakAtNextMdat := false

	_, err := mp4.ReadBoxStructure(r, func(h *mp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type.String() {
		case "moof":
			moofOffset = h.BoxInfo.Offset
			return h.Expand()

		case "traf":
			return h.Expand()

		case "tfhd":
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			tfhd = box.(*mp4.Tfhd)

		case "tfdt":
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			tfdt = box.(*mp4.Tfdt)

			m.setTrack(int(tfhd.TrackID))

		case "trun":
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			trun := box.(*mp4.Trun)

			dataOffset := moofOffset + uint64(trun.DataOffset)

			_, err = r.Seek(int64(dataOffset), io.SeekStart)
			if err != nil {
				return nil, err
			}

			muxerDTS := int64(tfdt.BaseMediaDecodeTimeV1) + int64(segmentStartOffsetMP4)

			for _, e := range trun.Entries {
				if muxerDTS >= int64(durationMP4) {
					breakAtNextMdat = true
					break
				}

				payload := make([]byte, e.SampleSize)
				_, err := io.ReadFull(r, payload)
				if err != nil {
					return nil, err
				}

				err = m.writeSample(
					muxerDTS,
					e.SampleCompositionTimeOffsetV1,
					(e.SampleFlags&sampleFlagIsNonSyncSample) != 0,
					payload,
				)
				if err != nil {
					return nil, err
				}

				muxerDTS += int64(e.SampleDuration)
			}

			m.writeFinalDTS(muxerDTS)

			if muxerDTS > maxMuxerDTS {
				maxMuxerDTS = muxerDTS
			}

		case "mdat":
			if breakAtNextMdat {
				return nil, errTerminated
			}
		}
		return nil, nil
	})
	if err != nil && !errors.Is(err, errTerminated) {
		return 0, err
	}

	return durationMp4ToGo(uint64(maxMuxerDTS), fmp4Timescale), nil
}
