package record

import (
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
)

type track struct {
	r         *Recorder
	initTrack *fmp4.InitTrack
	isVideo   bool

	nextSample *sample
}

func newTrack(
	r *Recorder,
	initTrack *fmp4.InitTrack,
	isVideo bool,
) *track {
	return &track{
		r:         r,
		initTrack: initTrack,
		isVideo:   isVideo,
	}
}

func (t *track) record(sample *sample) error {
	// wait the first video sample before setting hasVideo
	if t.isVideo {
		t.r.hasVideo = true
	}

	if t.r.currentSegment == nil {
		var err error
		t.r.currentSegment, err = newSegment(t.r, sample.dts)
		if err != nil {
			return err
		}
	}

	sample, t.nextSample = t.nextSample, sample
	if sample == nil {
		return nil
	}
	sample.Duration = uint32(durationGoToMp4(t.nextSample.dts-sample.dts, t.initTrack.TimeScale))

	err := t.r.currentSegment.record(t, sample)
	if err != nil {
		return err
	}

	if (!t.r.hasVideo || t.isVideo) &&
		!t.nextSample.IsNonSyncSample &&
		(t.nextSample.dts-t.r.currentSegment.startDTS) >= t.r.segmentDuration {
		err := t.r.currentSegment.close()
		if err != nil {
			return err
		}

		t.r.currentSegment, err = newSegment(t.r, t.nextSample.dts)
		if err != nil {
			return err
		}
	}

	return nil
}
