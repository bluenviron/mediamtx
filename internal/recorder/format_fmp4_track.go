package recorder

import (
	"fmt"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
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
	id        int
	clockRate uint32
	codec     mcodecs.Codec

	initTrack        *fmp4.InitTrack
	nextSample       *formatFMP4Sample
	startInitialized bool
	startDTS         time.Duration
	startNTP         time.Time
}

func (t *formatFMP4Track) initialize() {
	t.initTrack = &fmp4.InitTrack{
		ID:        t.id,
		TimeScale: t.clockRate,
		Codec:     t.codec,
	}
}

func (t *formatFMP4Track) write(sample *formatFMP4Sample) error {
	// wait the first video sample before setting hasVideo
	if t.initTrack.Codec.IsVideo() {
		t.f.hasVideo = true
	}

	sample, t.nextSample = t.nextSample, sample
	if sample == nil {
		return nil
	}

	dts := timestampToDuration(sample.dts, int(t.initTrack.TimeScale))

	if !t.startInitialized {
		t.startDTS = dts
		t.startNTP = sample.ntp
		t.startInitialized = true
	} else {
		drift := sample.ntp.Sub(t.startNTP) - (dts - t.startDTS)
		if drift < -ntpDriftTolerance || drift > ntpDriftTolerance {
			return fmt.Errorf("detected drift between recording duration and absolute time, resetting")
		}
	}

	duration := t.nextSample.dts - sample.dts
	if duration < 0 {
		t.nextSample.dts = sample.dts
		duration = 0
	} else {
		// This sample's duration is the gap until the next sample. When the
		// next sample's stream time has drifted from absolute time (e.g. an RTP
		// timestamp jump after packet loss) that gap can be arbitrarily large,
		// and it would inflate the segment duration by the size of the gap: the
		// drift is only caught on the following call, by which time the inflated
		// segment has already been flushed and reported. Zero the duration so
		// the segment isn't inflated; the drift detector above then resets the
		// recording when the next sample is processed.
		nextDTS := timestampToDuration(t.nextSample.dts, int(t.initTrack.TimeScale))
		nextDrift := t.nextSample.ntp.Sub(t.startNTP) - (nextDTS - t.startDTS)
		if nextDrift < -ntpDriftTolerance || nextDrift > ntpDriftTolerance {
			duration = 0
		}
	}

	sample.Duration = uint32(duration)

	if t.f.currentSegment == nil {
		t.f.currentSegment = &formatFMP4Segment{
			f:        t.f,
			startDTS: dts,
			startNTP: sample.ntp,
			number:   t.f.nextSegmentNumber,
		}
		t.f.currentSegment.initialize()
		t.f.nextSegmentNumber++
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
		err = t.f.currentSegment.close()
		if err != nil {
			return err
		}

		oldestNTP, oldestDTS := nextSegmentStartingPos(t.f.tracks)

		t.f.currentSegment = &formatFMP4Segment{
			f:        t.f,
			startDTS: oldestDTS,
			startNTP: oldestNTP,
			number:   t.f.nextSegmentNumber,
		}
		t.f.currentSegment.initialize()
		t.f.nextSegmentNumber++
	}

	return nil
}
