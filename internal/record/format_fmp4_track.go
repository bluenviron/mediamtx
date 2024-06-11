package record

import (
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
)

type formatFMP4Track struct {
	f         *formatFMP4
	initTrack *fmp4.InitTrack

	nextSample *sample
}

func (t *formatFMP4Track) write(sample *sample) error {
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
		t.f.currentSegment = &formatFMP4Segment{
			f:        t.f,
			startDTS: sample.dts,
			startNTP: sample.ntp,
		}
		t.f.currentSegment.initialize()
		// BaseTime is negative, this is not supported by fMP4. Reject the sample silently.
	} else if (sample.dts - t.f.currentSegment.startDTS) < 0 {
		return nil
	}

	err := t.f.currentSegment.write(t, sample)
	if err != nil {
		return err
	}

	if (!t.f.hasVideo || t.initTrack.Codec.IsVideo()) &&
		!t.nextSample.IsNonSyncSample &&
		(t.nextSample.dts-t.f.currentSegment.startDTS) >= t.f.a.agent.SegmentDuration {
		t.f.currentSegment.lastDTS = t.nextSample.dts
		err := t.f.currentSegment.close()
		if err != nil {
			return err
		}

		t.f.currentSegment = &formatFMP4Segment{
			f:        t.f,
			startDTS: t.nextSample.dts,
			startNTP: t.nextSample.ntp,
		}
		t.f.currentSegment.initialize()
	}

	return nil
}
