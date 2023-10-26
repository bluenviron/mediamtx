package record

import (
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
)

type recFormatFMP4Track struct {
	f         *recFormatFMP4
	initTrack *fmp4.InitTrack

	nextSample *sample
}

func newRecFormatFMP4Track(
	f *recFormatFMP4,
	initTrack *fmp4.InitTrack,
) *recFormatFMP4Track {
	return &recFormatFMP4Track{
		f:         f,
		initTrack: initTrack,
	}
}

func (t *recFormatFMP4Track) record(sample *sample) error {
	// wait the first video sample before setting hasVideo
	if t.initTrack.Codec.IsVideo() {
		t.f.hasVideo = true
	}

	if t.f.currentSegment == nil {
		t.f.currentSegment = newRecFormatFMP4Segment(t.f, sample.dts)
	}

	sample, t.nextSample = t.nextSample, sample
	if sample == nil {
		return nil
	}
	sample.Duration = uint32(durationGoToMp4(t.nextSample.dts-sample.dts, t.initTrack.TimeScale))

	err := t.f.currentSegment.record(t, sample)
	if err != nil {
		return err
	}

	if (!t.f.hasVideo || t.initTrack.Codec.IsVideo()) &&
		!t.nextSample.IsNonSyncSample &&
		(t.nextSample.dts-t.f.currentSegment.startDTS) >= t.f.a.wrapper.SegmentDuration {
		err := t.f.currentSegment.close()
		if err != nil {
			return err
		}

		t.f.currentSegment = newRecFormatFMP4Segment(t.f, t.nextSample.dts)
	}

	return nil
}
