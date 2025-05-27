package recorder

import (
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediamtx/internal/logger"
)

const (
	// this corresponds to concatenationTolerance
	maxBasetime = 1 * time.Second
)

// start next segment from the oldest next sample, in order to avoid negative basetimes (impossible) in fMP4.
// keep starting position within a certain distance from the newest next sample to avoid big basetimes.
func nextSegmentStartingPos(tracks []*formatFMP4Track) (time.Time, time.Duration) {
	var maxDTS time.Duration
	for _, track := range tracks {
		if track.nextSample != nil {
			dts := timestampToDuration(track.nextSample.dts, int(track.initTrack.TimeScale))
			if dts > maxDTS {
				maxDTS = dts
			}
		}
	}

	var oldestNTP time.Time
	oldestDTS := maxDTS

	for _, track := range tracks {
		if track.nextSample != nil {
			dts := timestampToDuration(track.nextSample.dts, int(track.initTrack.TimeScale))
			if (maxDTS-dts) <= maxBasetime && (dts <= oldestDTS) {
				oldestNTP = track.nextSample.ntp
				oldestDTS = dts
			}
		}
	}

	return oldestNTP, oldestDTS
}

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
	sample.Duration = uint32(t.nextSample.dts - sample.dts)

	dts := timestampToDuration(sample.dts, int(t.initTrack.TimeScale))

	if t.f.currentSegment == nil {
		t.f.currentSegment = &formatFMP4Segment{
			f:        t.f,
			startDTS: dts,
			startNTP: sample.ntp,
		}
		t.f.currentSegment.initialize()
	} else if (dts - t.f.currentSegment.startDTS) < 0 { // BaseTime is negative, this is not supported by fMP4
		t.f.ri.Log(logger.Warn, "sample of track %d received too late, discarding", t.initTrack.ID)
		return nil
	}

	err := t.f.currentSegment.write(t, sample, dts)
	if err != nil {
		return err
	}

	nextDTS := timestampToDuration(t.nextSample.dts, int(t.initTrack.TimeScale))

	if (!t.f.hasVideo || t.initTrack.Codec.IsVideo()) &&
		!t.nextSample.IsNonSyncSample &&
		(nextDTS-t.f.currentSegment.startDTS) >= t.f.ri.segmentDuration {
		err := t.f.currentSegment.close()
		if err != nil {
			return err
		}

		oldestNTP, oldestDTS := nextSegmentStartingPos(t.f.tracks)

		t.f.currentSegment = &formatFMP4Segment{
			f:        t.f,
			startDTS: oldestDTS,
			startNTP: oldestNTP,
		}
		t.f.currentSegment.initialize()
	}

	return nil
}
