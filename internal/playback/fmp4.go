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

	if !bytes.Equal(buf[4:], []byte{'f', 't', 'y', 'p'}) {
		return nil, fmt.Errorf("ftyp box not found")
	}

	ftypSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

	_, err = r.Seek(int64(ftypSize), io.SeekStart)
	if err != nil {
		return nil, err
	}

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

	buf = make([]byte, ftypSize+moovSize)

	_, err = io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

func seekAndMuxParts(
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

func muxParts(
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

	elapsed, err := seekAndMuxParts(f, init, minTime, maxTime, w)
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

	elapsed, err := muxParts(f, startTime, maxTime, w)
	if err != nil {
		return 0, err
	}

	return elapsed, nil
}
