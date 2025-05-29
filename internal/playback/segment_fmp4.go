package playback

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"time"

	"github.com/abema/go-mp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
)

const (
	sampleFlagIsNonSyncSample = 1 << 16
	concatenationTolerance    = 1 * time.Second
	maxBasetime               = 1 * time.Second
)

var errTerminated = errors.New("terminated")

type readSeekerAt interface {
	io.Reader
	io.Seeker
	io.ReaderAt
}

// try to guess where the next segment will start, using the same algorithm of nextSegmentStartingPos
func guessNextSegmentStartingPos(dtsPerTrack map[int]time.Duration) time.Duration {
	var maxElapsed time.Duration
	for _, dts := range dtsPerTrack {
		if dts > maxElapsed {
			maxElapsed = dts
		}
	}

	minElapsed := maxElapsed
	for _, dts := range dtsPerTrack {
		if (maxElapsed-dts) <= maxBasetime && (dts <= minElapsed) {
			minElapsed = dts
		}
	}

	return minElapsed
}

func durationGoToMp4(v time.Duration, timeScale uint32) int64 {
	timeScale64 := int64(timeScale)
	secs := v / time.Second
	dec := v % time.Second
	return int64(secs)*timeScale64 + int64(dec)*timeScale64/int64(time.Second)
}

func durationMp4ToGo(v int64, timeScale uint32) time.Duration {
	timeScale64 := int64(timeScale)
	secs := v / timeScale64
	dec := v % timeScale64
	return time.Duration(secs)*time.Second + time.Duration(dec)*time.Second/time.Duration(timeScale64)
}

func findInitTrack(tracks []*fmp4.InitTrack, id int) *fmp4.InitTrack {
	for _, track := range tracks {
		if track.ID == id {
			return track
		}
	}
	return nil
}

func segmentFMP4CanBeConcatenated(
	prevInit *fmp4.Init,
	prevEnd time.Time,
	curInit *fmp4.Init,
	curStart time.Time,
) bool {
	return reflect.DeepEqual(prevInit, curInit) &&
		!curStart.Before(prevEnd.Add(-concatenationTolerance)) &&
		!curStart.After(prevEnd.Add(concatenationTolerance))
}

func segmentFMP4ReadHeader(r io.ReadSeeker) (*fmp4.Init, time.Duration, error) {
	// check and skip ftyp

	buf := make([]byte, 8)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return nil, 0, err
	}

	if !bytes.Equal(buf[4:], []byte{'f', 't', 'y', 'p'}) {
		return nil, 0, fmt.Errorf("ftyp box not found")
	}

	ftypSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

	_, err = r.Seek(int64(ftypSize), io.SeekStart)
	if err != nil {
		return nil, 0, err
	}

	// check moov

	_, err = io.ReadFull(r, buf)
	if err != nil {
		return nil, 0, err
	}

	if !bytes.Equal(buf[4:], []byte{'m', 'o', 'o', 'v'}) {
		return nil, 0, fmt.Errorf("moov box not found")
	}

	moovSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

	// skip moov header

	_, err = r.Seek(8, io.SeekCurrent)
	if err != nil {
		return nil, 0, err
	}

	// read mvhd

	var mvhd mp4.Mvhd
	mvhdSize, err := mp4.Unmarshal(r, uint64(moovSize-8), &mvhd, mp4.Context{})
	if err != nil {
		return nil, 0, err
	}

	mvhdSize += 8                    // add mvhd header
	moovSize -= 8 + uint32(mvhdSize) // remove moov header and mvhd

	d := time.Duration(mvhd.DurationV0) * time.Second / time.Duration(mvhd.Timescale)

	// read tracks

	buf = make([]byte, uint64(moovSize))

	_, err = io.ReadFull(r, buf)
	if err != nil {
		return nil, 0, err
	}

	var init fmp4.Init
	err = init.Unmarshal(bytes.NewReader(buf))
	if err != nil {
		return nil, 0, err
	}

	return &init, d, nil
}

func segmentFMP4ReadDurationFromParts(
	r io.ReadSeeker,
	init *fmp4.Init,
) (time.Duration, error) {
	_, err := r.Seek(0, io.SeekStart)
	if err != nil {
		return 0, err
	}

	// check and skip ftyp

	buf := make([]byte, 8)
	_, err = io.ReadFull(r, buf)
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

	// check and skip moov

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
		var moofPos int64
		moofPos, err = r.Seek(0, io.SeekCurrent)
		if err != nil {
			return 0, err
		}

		_, err = io.ReadFull(r, buf)
		if err != nil {
			break
		}

		if !bytes.Equal(buf[4:], []byte{'m', 'o', 'o', 'f'}) {
			break
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
			break
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

	var maxDuration time.Duration

	// foreach traf

outer:
	for {
		_, err := io.ReadFull(r, buf)
		if err != nil {
			return 0, err
		}

		switch {
		case bytes.Equal(buf[4:], []byte{'t', 'r', 'a', 'f'}):
		case bytes.Equal(buf[4:], []byte{'m', 'd', 'a', 't'}):
			break outer
		default:
			return 0, fmt.Errorf("unexpected box %x", buf[4:8])
		}

		// parse tfhd

		_, err = io.ReadFull(r, buf)
		if err != nil {
			return 0, err
		}

		if !bytes.Equal(buf[4:], []byte{'t', 'f', 'h', 'd'}) {
			return 0, fmt.Errorf("tfhd box not found")
		}

		tfhdSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

		buf2 := make([]byte, tfhdSize-8)

		_, err = io.ReadFull(r, buf2)
		if err != nil {
			return 0, err
		}

		var tfhd mp4.Tfhd
		_, err = mp4.Unmarshal(bytes.NewReader(buf2), uint64(len(buf2)), &tfhd, mp4.Context{})
		if err != nil {
			return 0, fmt.Errorf("invalid tfhd box: %w", err)
		}

		track := findInitTrack(init.Tracks, int(tfhd.TrackID))
		if track == nil {
			return 0, fmt.Errorf("invalid track ID: %v", tfhd.TrackID)
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

		buf2 = make([]byte, tfdtSize-8)

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

		duration := int64(tfdt.BaseMediaDecodeTimeV1)

		for _, entry := range trun.Entries {
			duration += int64(entry.SampleDuration)
		}

		durationGo := durationMp4ToGo(duration, track.TimeScale)

		if durationGo > maxDuration {
			maxDuration = durationGo
		}
	}

	return maxDuration, nil
}

func segmentFMP4MuxParts(
	r readSeekerAt,
	start time.Duration,
	duration time.Duration,
	init *fmp4.Init,
	m muxer,
) (time.Duration, time.Duration, error) {
	var startMP4 int64
	var durationMP4 int64
	moofOffset := uint64(0)
	var tfhd *mp4.Tfhd
	var tfdt *mp4.Tfdt
	var timeScale uint32
	breakAtNextMdat := false
	var curTrackID int
	dtsPerTrack := make(map[int]time.Duration)
	var segDuration time.Duration

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

			track := findInitTrack(init.Tracks, int(tfhd.TrackID))
			if track == nil {
				return nil, fmt.Errorf("invalid track ID: %v", tfhd.TrackID)
			}

			curTrackID = int(tfhd.TrackID)
			m.setTrack(curTrackID)
			timeScale = track.TimeScale
			startMP4 = durationGoToMp4(start, track.TimeScale)
			durationMP4 = durationGoToMp4(duration, track.TimeScale)

		case "trun":
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			trun := box.(*mp4.Trun)

			dataOffset := moofOffset + uint64(trun.DataOffset)
			dts := int64(tfdt.BaseMediaDecodeTimeV1) + startMP4
			fmt.Println("FIDTS", dts)

			for _, e := range trun.Entries {
				if dts >= durationMP4 {
					breakAtNextMdat = true
					break
				}

				sampleOffset := dataOffset
				sampleSize := e.SampleSize

				err = m.writeSample(
					dts,
					e.SampleCompositionTimeOffsetV1,
					(e.SampleFlags&sampleFlagIsNonSyncSample) != 0,
					e.SampleSize,
					func() ([]byte, error) {
						payload := make([]byte, sampleSize)
						n, err2 := r.ReadAt(payload, int64(sampleOffset))
						if err2 != nil {
							return nil, err2
						}
						if n != int(sampleSize) {
							return nil, fmt.Errorf("partial read")
						}

						return payload, nil
					},
				)
				if err != nil {
					return nil, err
				}

				dataOffset += uint64(e.SampleSize)
				dts += int64(e.SampleDuration)
			}

			m.writeFinalDTS(dts)
			dtsPerTrack[curTrackID] = durationMp4ToGo(dts, timeScale)

			relDuration := durationMp4ToGo(dts-startMP4, timeScale)
			if relDuration > segDuration {
				segDuration = relDuration
			}

		case "mdat":
			if breakAtNextMdat {
				return nil, errTerminated
			}
		}
		return nil, nil
	})
	if err != nil && !errors.Is(err, errTerminated) {
		return 0, 0, err
	}

	nextSegmentStart := guessNextSegmentStartingPos(dtsPerTrack)
	return segDuration, nextSegmentStart, nil
}
