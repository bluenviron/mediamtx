package playback

import (
	"io"

	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4/seekablebuffer"
	"github.com/bluenviron/mediacommon/pkg/formats/mp4"
)

type muxerMP4TrackSample struct {
	mp4.HeaderTrackSample
	getPayload func() ([]byte, error)
}

type muxerMP4Track struct {
	id         int
	timeScale  uint32
	timeOffset int32
	codec      fmp4.Codec
	lastDTS    int64
	samples    []*muxerMP4TrackSample
}

func findTrackMP4(tracks []*muxerMP4Track, id int) *muxerMP4Track {
	for _, track := range tracks {
		if track.id == id {
			return track
		}
	}
	return nil
}

type muxerMP4 struct {
	w io.Writer

	tracks   []*muxerMP4Track
	curTrack *muxerMP4Track
}

func (w *muxerMP4) writeInit(init *fmp4.Init) {
	w.tracks = make([]*muxerMP4Track, len(init.Tracks))

	for i, track := range init.Tracks {
		w.tracks[i] = &muxerMP4Track{
			id:        track.ID,
			timeScale: track.TimeScale,
			codec:     track.Codec,
		}
	}
}

func (w *muxerMP4) setTrack(trackID int) {
	w.curTrack = findTrackMP4(w.tracks, trackID)
}

func (w *muxerMP4) writeSample(
	dts int64,
	ptsOffset int32,
	isNonSyncSample bool,
	payloadSize int,
	getPayload func() ([]byte, error),
) error {
	// remove GOPs before the GOP of the first frame
	if (dts < 0 || (dts >= 0 && w.curTrack.lastDTS < 0)) && !isNonSyncSample {
		w.curTrack.samples = nil
	}

	if w.curTrack.samples == nil {
		w.curTrack.timeOffset = int32(dts)
	} else {
		diff := dts - w.curTrack.lastDTS
		if diff < 0 {
			diff = 0
		}
		w.curTrack.samples[len(w.curTrack.samples)-1].Duration = uint32(diff)
	}

	// prevent warning "edit list: 1 Missing key frame while searching for timestamp: 0"
	if !isNonSyncSample {
		ptsOffset = 0
	}

	w.curTrack.samples = append(w.curTrack.samples, &muxerMP4TrackSample{
		HeaderTrackSample: mp4.HeaderTrackSample{
			PTSOffset:       ptsOffset,
			IsNonSyncSample: isNonSyncSample,
			PayloadSize:     payloadSize,
		},
		getPayload: getPayload,
	})
	w.curTrack.lastDTS = dts

	return nil
}

func (w *muxerMP4) writeFinalDTS(dts int64) {
	diff := dts - w.curTrack.lastDTS
	if diff < 0 {
		diff = 0
	}
	w.curTrack.samples[len(w.curTrack.samples)-1].Duration = uint32(diff)
}

func (w *muxerMP4) flush() error {
	h := mp4.Header{
		Tracks: make([]*mp4.HeaderTrack, len(w.tracks)),
	}

	for i, track := range w.tracks {
		h.Tracks[i] = &mp4.HeaderTrack{
			ID:         track.id,
			TimeScale:  track.timeScale,
			TimeOffset: track.timeOffset,
			Codec:      track.codec,
			Samples:    make([]*mp4.HeaderTrackSample, len(track.samples)),
		}

		for j, sample := range track.samples {
			h.Tracks[i].Samples[j] = &sample.HeaderTrackSample
		}
	}

	var outBuf seekablebuffer.Buffer

	err := h.Marshal(&outBuf)
	if err != nil {
		return err
	}

	_, err = w.w.Write(outBuf.Bytes())
	if err != nil {
		return err
	}

	mdatSize := uint32(8)

	for _, track := range w.tracks {
		for _, sa := range track.samples {
			mdatSize += uint32(sa.PayloadSize)
		}
	}

	_, err = w.w.Write([]byte{byte(mdatSize >> 24), byte(mdatSize >> 16), byte(mdatSize >> 8), byte(mdatSize)})
	if err != nil {
		return err
	}

	_, err = w.w.Write([]byte{'m', 'd', 'a', 't'})
	if err != nil {
		return err
	}

	for _, track := range w.tracks {
		for _, sa := range track.samples {
			pl, err := sa.getPayload()
			if err != nil {
				return err
			}

			_, err = w.w.Write(pl)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
