package record

import (
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
)

type track struct {
	r         *Agent
	initTrack *fmp4.InitTrack

	nextSample *sample
}

func newTrack(
	r *Agent,
	initTrack *fmp4.InitTrack,
) *track {
	return &track{
		r:         r,
		initTrack: initTrack,
	}
}

func (t *track) record(sample *sample) error {
	// wait the first video sample before setting hasVideo
	if t.initTrack.Codec.IsVideo() {
		t.r.hasVideo = true
	}

	if t.r.currentSegment == nil {
		t.r.currentSegment = newSegment(t.r, sample.dts)
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

	if (!t.r.hasVideo || t.initTrack.Codec.IsVideo()) &&
		!t.nextSample.IsNonSyncSample &&
		(t.nextSample.dts-t.r.currentSegment.startDTS) >= t.r.segmentDuration {
		err := t.r.currentSegment.close()
		if err != nil {
			return err
		}

		t.r.currentSegment = newSegment(t.r, t.nextSample.dts)
	}

	return nil
}
