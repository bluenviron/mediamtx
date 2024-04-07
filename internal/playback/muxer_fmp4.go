package playback

import (
	"io"

	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4/seekablebuffer"
)

type muxerFMP4Track struct {
	started bool
	fmp4.PartTrack
}

func findTrack(tracks []*muxerFMP4Track, id int) *muxerFMP4Track {
	for _, track := range tracks {
		if track.ID == id {
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
			PartTrack: fmp4.PartTrack{
				ID: trackID,
			},
		}
		w.tracks = append(w.tracks, w.curTrack)
	}
}

func (w *muxerFMP4) writeSample(normalizedElapsed int64, sample *fmp4.PartSample) {
	if !w.curTrack.started {
		if normalizedElapsed >= 0 {
			w.curTrack.started = true
			w.curTrack.BaseTime = uint64(normalizedElapsed)

			if !sample.IsNonSyncSample {
				w.curTrack.Samples = []*fmp4.PartSample{sample}
			} else {
				w.curTrack.Samples = append(w.curTrack.Samples, sample)
			}
		} else {
			sample.Duration = 0
			sample.PTSOffset = 0

			if !sample.IsNonSyncSample {
				w.curTrack.Samples = []*fmp4.PartSample{sample}
			} else {
				w.curTrack.Samples = append(w.curTrack.Samples, sample)
			}
		}
	} else {
		if w.curTrack.Samples == nil {
			w.curTrack.BaseTime = uint64(normalizedElapsed)
		}
		w.curTrack.Samples = append(w.curTrack.Samples, sample)
	}
}

func (w *muxerFMP4) flush() error {
	var part fmp4.Part

	for _, track := range w.tracks {
		if track.started && track.Samples != nil {
			part.Tracks = append(part.Tracks, &track.PartTrack)
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

	for _, track := range w.tracks {
		if track.started {
			track.Samples = nil
		}
	}

	return nil
}
