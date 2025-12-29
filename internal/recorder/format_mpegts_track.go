package recorder

import (
	"fmt"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	tscodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts/codecs"
)

type formatMPEGTSTrack struct {
	f     *formatMPEGTS
	codec tscodecs.Codec

	track            *mpegts.Track
	startInitialized bool
	startDTS         time.Duration
	startNTP         time.Time
}

func (t *formatMPEGTSTrack) initialize() {
	t.track = &mpegts.Track{
		Codec: t.codec,
	}
}

func (t *formatMPEGTSTrack) write(
	dts time.Duration,
	ntp time.Time,
	randomAccess bool,
	cb func(track *mpegts.Track) error,
) error {
	isVideo := t.track.Codec.IsVideo()

	if isVideo {
		t.f.hasVideo = true
	}

	if !t.startInitialized {
		t.startDTS = dts
		t.startNTP = ntp
		t.startInitialized = true
	} else {
		drift := ntp.Sub(t.startNTP) - (dts - t.startDTS)
		if drift < -ntpDriftTolerance || drift > ntpDriftTolerance {
			return fmt.Errorf("detected drift between recording duration and absolute time, resetting")
		}
	}

	switch {
	case t.f.currentSegment == nil:
		t.f.currentSegment = &formatMPEGTSSegment{
			pathFormat2:       t.f.ri.pathFormat2,
			flush:             t.f.bw.Flush,
			onSegmentCreate:   t.f.ri.onSegmentCreate,
			onSegmentComplete: t.f.ri.onSegmentComplete,
			startDTS:          dts,
			startNTP:          ntp,
			log:               t.f.ri,
		}
		t.f.currentSegment.initialize()
		t.f.dw.setTarget(t.f.currentSegment)

	case (!t.f.hasVideo || isVideo) &&
		randomAccess &&
		(dts-t.f.currentSegment.startDTS) >= t.f.ri.segmentDuration:
		t.f.currentSegment.lastDTS = dts
		err := t.f.currentSegment.close()
		if err != nil {
			return err
		}

		t.f.currentSegment = &formatMPEGTSSegment{
			pathFormat2:       t.f.ri.pathFormat2,
			flush:             t.f.bw.Flush,
			onSegmentCreate:   t.f.ri.onSegmentCreate,
			onSegmentComplete: t.f.ri.onSegmentComplete,
			startDTS:          dts,
			startNTP:          ntp,
			log:               t.f.ri,
		}
		t.f.currentSegment.initialize()
		t.f.dw.setTarget(t.f.currentSegment)

	case (dts - t.f.currentSegment.lastFlush) >= t.f.ri.partDuration:
		err := t.f.bw.Flush()
		if err != nil {
			return err
		}

		t.f.currentSegment.lastFlush = dts
	}

	t.f.currentSegment.lastDTS = dts

	return cb(t.track)
}
