package playback

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"time"

	amp4 "github.com/abema/go-mp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"

	"github.com/bluenviron/mediamtx/internal/recordstore"
)

const (
	sampleFlagIsNonSyncSample = 1 << 16
	concatenationTolerance    = 1 * time.Second
)

var errTerminated = errors.New("terminated")

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

func findMtxi(userData []amp4.IBox) *recordstore.Mtxi {
	for _, box := range userData {
		if i, ok := box.(*recordstore.Mtxi); ok {
			return i
		}
	}
	return nil
}

func segmentFMP4TracksAreEqual(tracks1 []*fmp4.InitTrack, tracks2 []*fmp4.InitTrack) bool {
	if len(tracks1) != len(tracks2) {
		return false
	}

	for i, track1 := range tracks1 {
		track2 := tracks2[i]

		if track1.ID != track2.ID ||
			track1.TimeScale != track2.TimeScale ||
			reflect.TypeOf(track1.Codec) != reflect.TypeOf(track2.Codec) {
			return false
		}
	}

	return true
}

func segmentFMP4CanBeConcatenated(
	prevInit *fmp4.Init,
	prevEnd time.Time,
	curInit *fmp4.Init,
	curStart time.Time,
) bool {
	mtxi1 := findMtxi(prevInit.UserData)
	mtxi2 := findMtxi(curInit.UserData)

	switch {
	case mtxi1 == nil && mtxi2 != nil:
		return false

	case mtxi1 != nil && mtxi2 == nil:
		return false

	case mtxi1 == nil && mtxi2 == nil: // legacy method
		return segmentFMP4TracksAreEqual(prevInit.Tracks, curInit.Tracks) &&
			!curStart.Before(prevEnd.Add(-concatenationTolerance)) &&
			!curStart.After(prevEnd.Add(concatenationTolerance))

	default:
		return bytes.Equal(mtxi1.StreamID[:], mtxi2.StreamID[:]) &&
			(mtxi1.SegmentNumber+1) == mtxi2.SegmentNumber
	}
}

func segmentFMP4MuxParts(
	r readSeekerAt,
	startDTS time.Duration,
	duration time.Duration,
	tracks []*fmp4.InitTrack,
	m muxer,
) (time.Duration, error) {
	var startDTSMP4 int64
	var durationMP4 int64
	moofOffset := uint64(0)
	var tfhd *amp4.Tfhd
	var tfdt *amp4.Tfdt
	var timeScale uint32
	var segmentDuration time.Duration
	breakAtNextMdat := false

	_, err := amp4.ReadBoxStructure(r, func(h *amp4.ReadHandle) (any, error) {
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
			tfhd = box.(*amp4.Tfhd)

		case "tfdt":
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			tfdt = box.(*amp4.Tfdt)

			track := findInitTrack(tracks, int(tfhd.TrackID))
			if track == nil {
				return nil, fmt.Errorf("invalid track ID: %v", tfhd.TrackID)
			}

			m.setTrack(int(tfhd.TrackID))
			timeScale = track.TimeScale
			startDTSMP4 = durationGoToMp4(startDTS, track.TimeScale)
			durationMP4 = durationGoToMp4(duration, track.TimeScale)

		case "trun":
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			trun := box.(*amp4.Trun)

			dataOffset := moofOffset + uint64(trun.DataOffset)
			dts := int64(tfdt.BaseMediaDecodeTimeV1) + startDTSMP4

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

			segmentElapsed := durationMp4ToGo(dts-startDTSMP4, timeScale)

			if segmentElapsed > segmentDuration {
				segmentDuration = segmentElapsed
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

	return segmentDuration, nil
}

type readSeekerAt interface {
	io.Reader
	io.Seeker
	io.ReaderAt
}
