package playback

import (
	"io"

	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4/seekablebuffer"
)

type muxerFMP4Track struct {
	started  bool
	id       int
	firstDTS uint64
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
			id: trackID,
		}
		w.tracks = append(w.tracks, w.curTrack)
	}
}

func (w *muxerFMP4) writeSample(dts int64, ptsOffset int32, isNonSyncSample bool, payload []byte) {
	if !w.curTrack.started {
		if dts >= 0 {
			w.curTrack.started = true
			w.curTrack.firstDTS = uint64(dts)

			if !isNonSyncSample {
				w.curTrack.samples = []*fmp4.PartSample{{
					PTSOffset:       ptsOffset,
					IsNonSyncSample: isNonSyncSample,
					Payload:         payload,
				}}
			} else {
				w.curTrack.samples = append(w.curTrack.samples, &fmp4.PartSample{
					PTSOffset:       ptsOffset,
					IsNonSyncSample: isNonSyncSample,
					Payload:         payload,
				})
			}
			w.curTrack.lastDTS = dts
		} else {
			ptsOffset = 0

			if !isNonSyncSample {
				w.curTrack.samples = []*fmp4.PartSample{{
					PTSOffset:       ptsOffset,
					IsNonSyncSample: isNonSyncSample,
					Payload:         payload,
				}}
			} else {
				w.curTrack.samples = append(w.curTrack.samples, &fmp4.PartSample{
					PTSOffset:       ptsOffset,
					IsNonSyncSample: isNonSyncSample,
					Payload:         payload,
				})
			}
		}
	} else {
		if w.curTrack.samples == nil {
			w.curTrack.firstDTS = uint64(dts)
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
	}
}

func (w *muxerFMP4) writeFinalDTS(dts int64) {
	if w.curTrack.started && w.curTrack.samples != nil {
		diff := dts - w.curTrack.lastDTS
		if diff < 0 {
			diff = 0
		}

		w.curTrack.samples[len(w.curTrack.samples)-1].Duration = uint32(diff)
	}
}

func (w *muxerFMP4) flush2(final bool) error {
	var part fmp4.Part

	for _, track := range w.tracks {
		if track.started && (len(track.samples) > 1 || (final && len(track.samples) != 0)) {
			var samples []*fmp4.PartSample
			if !final {
				samples = track.samples[:len(track.samples)-1]
			} else {
				samples = track.samples
			}

			part.Tracks = append(part.Tracks, &fmp4.PartTrack{
				ID:       track.id,
				BaseTime: track.firstDTS,
				Samples:  samples,
			})

			if !final {
				track.samples = track.samples[len(track.samples)-1:]
				track.firstDTS = uint64(track.lastDTS)
			} else {
				track.samples = nil
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
	return w.flush2(false)
}

func (w *muxerFMP4) finalFlush() error {
	return w.flush2(true)
}
