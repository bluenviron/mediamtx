package playback

import (
	"io"
	"time"

	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4/seekablebuffer"
)

var partSize = durationGoToMp4(1*time.Second, fmp4Timescale)

type muxerFMP4Track struct {
	id       int
	firstDTS int64
	lastDTS  int64
	samples  []*fmp4.PartSample
}

func findTrack(tracks []*muxerFMP4Track, id int) *muxerFMP4Track {
	for _, track := range tracks {
		if track.id == id {
			return track
		}
	}
	return nil
}

type muxerFMP4 struct {
	w io.Writer

	init               []byte
	nextSequenceNumber uint32
	tracks             []*muxerFMP4Track
	curTrack           *muxerFMP4Track
	outBuf             seekablebuffer.Buffer
}

func (w *muxerFMP4) writeInit(init []byte) {
	w.init = init
}

func (w *muxerFMP4) setTrack(trackID int) {
	w.curTrack = findTrack(w.tracks, trackID)
	if w.curTrack == nil {
		w.curTrack = &muxerFMP4Track{
			id:       trackID,
			firstDTS: -1,
		}
		w.tracks = append(w.tracks, w.curTrack)
	}
}

func (w *muxerFMP4) writeSample(dts int64, ptsOffset int32, isNonSyncSample bool, payload []byte) error {
	if dts >= 0 {
		if w.curTrack.firstDTS < 0 {
			w.curTrack.firstDTS = dts

			// reset GOP preceding the first frame
			if !isNonSyncSample {
				w.curTrack.samples = nil
			}
		} else {
			diff := dts - w.curTrack.lastDTS
			if diff < 0 {
				diff = 0
			}
			w.curTrack.samples[len(w.curTrack.samples)-1].Duration = uint32(diff)
		}

		w.curTrack.samples = append(w.curTrack.samples, &fmp4.PartSample{
			PTSOffset:       ptsOffset,
			IsNonSyncSample: isNonSyncSample,
			Payload:         payload,
		})
		w.curTrack.lastDTS = dts

		if (w.curTrack.lastDTS - w.curTrack.firstDTS) > int64(partSize) {
			err := w.innerFlush(false)
			if err != nil {
				return err
			}
		}
	} else {
		// store GOP preceding the first frame, with PTSOffset = 0 and Duration = 0
		if !isNonSyncSample {
			w.curTrack.samples = []*fmp4.PartSample{{
				IsNonSyncSample: isNonSyncSample,
				Payload:         payload,
			}}
		} else {
			w.curTrack.samples = append(w.curTrack.samples, &fmp4.PartSample{
				IsNonSyncSample: isNonSyncSample,
				Payload:         payload,
			})
		}
	}

	return nil
}

func (w *muxerFMP4) writeFinalDTS(dts int64) {
	if w.curTrack.firstDTS >= 0 {
		diff := dts - w.curTrack.lastDTS
		if diff < 0 {
			diff = 0
		}
		w.curTrack.samples[len(w.curTrack.samples)-1].Duration = uint32(diff)
	}
}

func (w *muxerFMP4) innerFlush(final bool) error {
	var part fmp4.Part

	for _, track := range w.tracks {
		if track.firstDTS >= 0 && (len(track.samples) > 1 || (final && len(track.samples) != 0)) {
			// do not write the final sample
			// in order to allow changing its duration to compensate NTP-DTS differences
			var samples []*fmp4.PartSample
			if !final {
				samples = track.samples[:len(track.samples)-1]
			} else {
				samples = track.samples
			}

			part.Tracks = append(part.Tracks, &fmp4.PartTrack{
				ID:       track.id,
				BaseTime: uint64(track.firstDTS),
				Samples:  samples,
			})

			if !final {
				track.samples = track.samples[len(track.samples)-1:]
				track.firstDTS = track.lastDTS
			}
		}
	}

	if part.Tracks != nil {
		part.SequenceNumber = w.nextSequenceNumber
		w.nextSequenceNumber++

		if w.init != nil {
			_, err := w.w.Write(w.init)
			if err != nil {
				return err
			}
			w.init = nil
		}

		err := part.Marshal(&w.outBuf)
		if err != nil {
			return err
		}

		_, err = w.w.Write(w.outBuf.Bytes())
		if err != nil {
			return err
		}

		w.outBuf.Reset()
	}

	return nil
}

func (w *muxerFMP4) flush() error {
	return w.innerFlush(true)
}
