package recordstore

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"time"

	amp4 "github.com/abema/go-mp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
)

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

// SegmentFMP4ReadHeader reads the header of a recording segment in fMP4 format.
func SegmentFMP4ReadHeader(r io.ReadSeeker) (*fmp4.Init, time.Duration, error) {
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

	var mvhd amp4.Mvhd
	_, err = amp4.Unmarshal(r, uint64(moovSize-8), &mvhd, amp4.Context{})
	if err != nil {
		return nil, 0, err
	}

	d := time.Duration(mvhd.DurationV0) * time.Second / time.Duration(mvhd.Timescale)

	// read ftyp and moov

	_, err = r.Seek(0, io.SeekStart)
	if err != nil {
		return nil, 0, err
	}

	buf = make([]byte, uint64(ftypSize+moovSize))

	_, err = io.ReadFull(r, buf)
	if err != nil {
		return nil, 0, err
	}

	// pass ftyp and moov to fmp4.Init

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

	var maxElapsed time.Duration

	// foreach traf

outer:
	for {
		_, err = io.ReadFull(r, buf)
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

		var tfhd amp4.Tfhd
		_, err = amp4.Unmarshal(bytes.NewReader(buf2), uint64(len(buf2)), &tfhd, amp4.Context{})
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

		var tfdt amp4.Tfdt
		_, err = amp4.Unmarshal(bytes.NewReader(buf2), uint64(len(buf2)), &tfdt, amp4.Context{})
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

		var trun amp4.Trun
		_, err = amp4.Unmarshal(bytes.NewReader(buf2), uint64(len(buf2)), &trun, amp4.Context{})
		if err != nil {
			return 0, fmt.Errorf("invalid trun box: %w", err)
		}

		elapsed := int64(tfdt.BaseMediaDecodeTimeV1)

		for _, entry := range trun.Entries {
			elapsed += int64(entry.SampleDuration)
		}

		elapsedGo := durationMp4ToGo(elapsed, track.TimeScale)

		if elapsedGo > maxElapsed {
			maxElapsed = elapsedGo
		}
	}

	return maxElapsed, nil
}

// SegmentFMP4ReadHeaderAndDuration reads the header and duration of a recording segment in fMP4 format.
func SegmentFMP4ReadHeaderAndDuration(fpath string) (*fmp4.Init, time.Duration, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	init, duration, err := SegmentFMP4ReadHeader(f)
	if err != nil {
		return nil, 0, err
	}

	// if duration is not present in the header, compute it
	// by parsing each part
	if duration == 0 {
		duration, err = segmentFMP4ReadDurationFromParts(f, init)
		if err != nil {
			return nil, 0, err
		}
	}

	return init, duration, nil
}
