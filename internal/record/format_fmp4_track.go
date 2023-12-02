package record

import (
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
)

type formatFMP4Track struct {
	f         *formatFMP4
	initTrack *fmp4.InitTrack

	nextSample *sample
}

func newFormatFMP4Track(
	f *formatFMP4,
	initTrack *fmp4.InitTrack,
) *formatFMP4Track {
	return &formatFMP4Track{
		f:         f,
		initTrack: initTrack,
	}
}

func (t *formatFMP4Track) record(sample *sample) error {
	// wait the first video sample before setting hasVideo
	if t.initTrack.Codec.IsVideo() {
		t.f.hasVideo = true
	}

	sample, t.nextSample = t.nextSample, sample
	if sample == nil {
		return nil
	}
	sample.Duration = uint32(durationGoToMp4(t.nextSample.dts-sample.dts, t.initTrack.TimeScale))

	if t.f.currentSegment == nil {
		t.f.currentSegment = newFormatFMP4Segment(t.f, sample.dts)
		// BaseTime is negative, this is not supported by fMP4. Reject the sample silently.
	} else if (sample.dts - t.f.currentSegment.startDTS) < 0 {
		return nil
	}

	err := t.f.currentSegment.record(t, sample)
	if err != nil {
		return err
	}

	if (!t.f.hasVideo || t.initTrack.Codec.IsVideo()) &&
		!t.nextSample.IsNonSyncSample &&
		(t.nextSample.dts-t.f.currentSegment.startDTS) >= t.f.a.agent.SegmentDuration {
		err := t.f.currentSegment.close()
		if err != nil {
			return err
		}

		t.f.currentSegment = newFormatFMP4Segment(t.f, t.nextSample.dts)
	}

	return nil
}
