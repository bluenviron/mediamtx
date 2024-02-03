package playback

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/abema/go-mp4"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4/seekablebuffer"
)

const (
	sampleFlagIsNonSyncSample = 1 << 16
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

func fmp4ReadInit(r io.ReadSeeker) ([]byte, error) {
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

func fmp4SeekAndMuxParts(
	r io.ReadSeeker,
	init []byte,
	minTime time.Duration,
	maxTime time.Duration,
	w io.Writer,
) (time.Duration, error) {
	minTimeMP4 := durationGoToMp4(minTime, 90000)
	maxTimeMP4 := durationGoToMp4(maxTime, 90000)
	moofOffset := uint64(0)
	var tfhd *mp4.Tfhd
	var tfdt *mp4.Tfdt
	var outPart *fmp4.Part
	var outTrack *fmp4.PartTrack
	var outBuf seekablebuffer.Buffer
	elapsed := uint64(0)
	initWritten := false
	firstSampleWritten := make(map[uint32]struct{})
	gop := make(map[uint32][]*fmp4.PartSample)

	_, err := mp4.ReadBoxStructure(r, func(h *mp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type.String() {
		case "moof":
			moofOffset = h.BoxInfo.Offset
			outPart = &fmp4.Part{}
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

			if tfdt.BaseMediaDecodeTimeV1 >= maxTimeMP4 {
				return nil, errTerminated
			}

			outTrack = &fmp4.PartTrack{ID: int(tfhd.TrackID)}

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

			elapsed = tfdt.BaseMediaDecodeTimeV1
			baseTimeSet := false

			for _, e := range trun.Entries {
				payload := make([]byte, e.SampleSize)
				_, err := io.ReadFull(r, payload)
				if err != nil {
					return nil, err
				}

				if elapsed >= maxTimeMP4 {
					break
				}

				isRandom := (e.SampleFlags & sampleFlagIsNonSyncSample) == 0
				_, fsw := firstSampleWritten[tfhd.TrackID]

				sa := &fmp4.PartSample{
					Duration:        e.SampleDuration,
					PTSOffset:       e.SampleCompositionTimeOffsetV1,
					IsNonSyncSample: !isRandom,
					Payload:         payload,
				}

				if !fsw {
					if isRandom {
						gop[tfhd.TrackID] = []*fmp4.PartSample{sa}
					} else {
						gop[tfhd.TrackID] = append(gop[tfhd.TrackID], sa)
					}
				}

				if elapsed >= minTimeMP4 {
					if !baseTimeSet {
						outTrack.BaseTime = elapsed - minTimeMP4

						if !fsw {
							if !isRandom {
								for _, sa2 := range gop[tfhd.TrackID][:len(gop[tfhd.TrackID])-1] {
									sa2.Duration = 0
									sa2.PTSOffset = 0
									outTrack.Samples = append(outTrack.Samples, sa2)
								}
							}

							delete(gop, tfhd.TrackID)
							firstSampleWritten[tfhd.TrackID] = struct{}{}
						}
					}

					outTrack.Samples = append(outTrack.Samples, sa)
				}

				elapsed += uint64(e.SampleDuration)
			}

			if outTrack.Samples != nil {
				outPart.Tracks = append(outPart.Tracks, outTrack)
			}

			outTrack = nil

		case "mdat":
			if outPart.Tracks != nil {
				if !initWritten {
					initWritten = true
					_, err := w.Write(init)
					if err != nil {
						return nil, err
					}
				}

				err := outPart.Marshal(&outBuf)
				if err != nil {
					return nil, err
				}

				_, err = w.Write(outBuf.Bytes())
				if err != nil {
					return nil, err
				}

				outBuf.Reset()
			}

			outPart = nil
		}
		return nil, nil
	})
	if err != nil && !errors.Is(err, errTerminated) {
		return 0, err
	}

	if !initWritten {
		return 0, errNoSegmentsFound
	}

	elapsed -= minTimeMP4

	return durationMp4ToGo(elapsed, 90000), nil
}

func fmp4MuxParts(
	r io.ReadSeeker,
	startTime time.Duration,
	maxTime time.Duration,
	w io.Writer,
) (time.Duration, error) {
	maxTimeMP4 := durationGoToMp4(maxTime, 90000)
	moofOffset := uint64(0)
	var tfhd *mp4.Tfhd
	var tfdt *mp4.Tfdt
	var outPart *fmp4.Part
	var outTrack *fmp4.PartTrack
	var outBuf seekablebuffer.Buffer
	elapsed := uint64(0)

	_, err := mp4.ReadBoxStructure(r, func(h *mp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type.String() {
		case "moof":
			moofOffset = h.BoxInfo.Offset
			outPart = &fmp4.Part{}
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

			if tfdt.BaseMediaDecodeTimeV1 >= maxTimeMP4 {
				return nil, errTerminated
			}

			outTrack = &fmp4.PartTrack{
				ID:       int(tfhd.TrackID),
				BaseTime: tfdt.BaseMediaDecodeTimeV1 + durationGoToMp4(startTime, 90000),
			}

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

			elapsed = tfdt.BaseMediaDecodeTimeV1

			for _, e := range trun.Entries {
				payload := make([]byte, e.SampleSize)
				_, err := io.ReadFull(r, payload)
				if err != nil {
					return nil, err
				}

				if elapsed >= maxTimeMP4 {
					break
				}

				isRandom := (e.SampleFlags & sampleFlagIsNonSyncSample) == 0

				sa := &fmp4.PartSample{
					Duration:        e.SampleDuration,
					PTSOffset:       e.SampleCompositionTimeOffsetV1,
					IsNonSyncSample: !isRandom,
					Payload:         payload,
				}

				outTrack.Samples = append(outTrack.Samples, sa)

				elapsed += uint64(e.SampleDuration)
			}

			if outTrack.Samples != nil {
				outPart.Tracks = append(outPart.Tracks, outTrack)
			}

			outTrack = nil

		case "mdat":
			if outPart.Tracks != nil {
				err := outPart.Marshal(&outBuf)
				if err != nil {
					return nil, err
				}

				_, err = w.Write(outBuf.Bytes())
				if err != nil {
					return nil, err
				}

				outBuf.Reset()
			}

			outPart = nil
		}
		return nil, nil
	})
	if err != nil && !errors.Is(err, errTerminated) {
		return 0, err
	}

	return durationMp4ToGo(elapsed, 90000), nil
}

func fmp4SeekAndMux(
	fpath string,
	minTime time.Duration,
	maxTime time.Duration,
	w io.Writer,
) (time.Duration, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	init, err := fmp4ReadInit(f)
	if err != nil {
		return 0, err
	}

	elapsed, err := fmp4SeekAndMuxParts(f, init, minTime, maxTime, w)
	if err != nil {
		return 0, err
	}

	return elapsed, nil
}

func fmp4Mux(
	fpath string,
	startTime time.Duration,
	maxTime time.Duration,
	w io.Writer,
) (time.Duration, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	return fmp4MuxParts(f, startTime, maxTime, w)
}

func fmp4Duration(fpath string) (time.Duration, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	// find and skip ftyp

	buf := make([]byte, 8)
	_, err = io.ReadFull(f, buf)
	if err != nil {
		return 0, err
	}

	if !bytes.Equal(buf[4:], []byte{'f', 't', 'y', 'p'}) {
		return 0, fmt.Errorf("ftyp box not found")
	}

	ftypSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

	_, err = f.Seek(int64(ftypSize), io.SeekStart)
	if err != nil {
		return 0, err
	}

	// find and skip moov

	_, err = io.ReadFull(f, buf)
	if err != nil {
		return 0, err
	}

	if !bytes.Equal(buf[4:], []byte{'m', 'o', 'o', 'v'}) {
		return 0, fmt.Errorf("moov box not found")
	}

	moovSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

	_, err = f.Seek(int64(moovSize)-8, io.SeekCurrent)
	if err != nil {
		return 0, err
	}

	// find last valid moof and mdat

	lastMoofPos := int64(-1)

	for {
		moofPos, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			return 0, err
		}

		_, err = io.ReadFull(f, buf)
		if err != nil {
			break
		}

		if !bytes.Equal(buf[4:], []byte{'m', 'o', 'o', 'f'}) {
			return 0, fmt.Errorf("moof box not found")
		}

		moofSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

		_, err = f.Seek(int64(moofSize)-8, io.SeekCurrent)
		if err != nil {
			break
		}

		_, err = io.ReadFull(f, buf)
		if err != nil {
			break
		}

		if !bytes.Equal(buf[4:], []byte{'m', 'd', 'a', 't'}) {
			return 0, fmt.Errorf("mdat box not found")
		}

		mdatSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

		_, err = f.Seek(int64(mdatSize)-8, io.SeekCurrent)
		if err != nil {
			break
		}

		lastMoofPos = moofPos
	}

	if lastMoofPos < 0 {
		return 0, fmt.Errorf("no moof boxes found")
	}

	// open last moof

	_, err = f.Seek(lastMoofPos+8, io.SeekStart)
	if err != nil {
		return 0, err
	}

	_, err = io.ReadFull(f, buf)
	if err != nil {
		return 0, err
	}

	// skip mfhd

	if !bytes.Equal(buf[4:], []byte{'m', 'f', 'h', 'd'}) {
		return 0, fmt.Errorf("mfhd box not found")
	}

	_, err = f.Seek(8, io.SeekCurrent)
	if err != nil {
		return 0, err
	}

	maxElapsed := uint64(0)

	// foreach traf

	for {
		_, err := io.ReadFull(f, buf)
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

		_, err = io.ReadFull(f, buf)
		if err != nil {
			return 0, err
		}

		if !bytes.Equal(buf[4:], []byte{'t', 'f', 'h', 'd'}) {
			return 0, fmt.Errorf("tfhd box not found")
		}

		tfhdSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

		_, err = f.Seek(int64(tfhdSize)-8, io.SeekCurrent)
		if err != nil {
			return 0, err
		}

		// parse tfdt

		_, err = io.ReadFull(f, buf)
		if err != nil {
			return 0, err
		}

		if !bytes.Equal(buf[4:], []byte{'t', 'f', 'd', 't'}) {
			return 0, fmt.Errorf("tfdt box not found")
		}

		tfdtSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

		buf2 := make([]byte, tfdtSize-8)

		_, err = io.ReadFull(f, buf2)
		if err != nil {
			return 0, err
		}

		var tfdt mp4.Tfdt
		_, err = mp4.Unmarshal(bytes.NewReader(buf2), uint64(len(buf2)), &tfdt, mp4.Context{})
		if err != nil {
			return 0, fmt.Errorf("invalid tfdt box: %w", err)
		}

		elapsed := tfdt.BaseMediaDecodeTimeV1

		// parse trun

		_, err = io.ReadFull(f, buf)
		if err != nil {
			return 0, err
		}

		if !bytes.Equal(buf[4:], []byte{'t', 'r', 'u', 'n'}) {
			return 0, fmt.Errorf("trun box not found")
		}

		trunSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

		buf2 = make([]byte, trunSize-8)

		_, err = io.ReadFull(f, buf2)
		if err != nil {
			return 0, err
		}

		var trun mp4.Trun
		_, err = mp4.Unmarshal(bytes.NewReader(buf2), uint64(len(buf2)), &trun, mp4.Context{})
		if err != nil {
			return 0, fmt.Errorf("invalid trun box: %w", err)
		}

		for _, entry := range trun.Entries {
			elapsed += uint64(entry.SampleDuration)
		}

		if elapsed > maxElapsed {
			maxElapsed = elapsed
		}
	}

	return durationMp4ToGo(maxElapsed, 90000), nil
}
