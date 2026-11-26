package recorder

import (
	"fmt"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/logger"
)

const (
	// this corresponds to concatenationTolerance
	maxBasetime = 1 * time.Second
)

// start next segment from the oldest next sample, in order to avoid negative basetimes (impossible) in fMP4.
// keep starting position within a certain distance from the reference sample to avoid big basetimes.
//
// The reference is the sync sample that triggered the switch (sync), so that sample always starts
// inside the new segment: a track whose next sample is older than it by at most maxBasetime moves
// the start back, and a track ahead of it just gets a positive basetime. Upstream used the newest
// next sample of any track as the reference, so when audio decoded more than maxBasetime ahead of
// video the start skipped past the video's keyframe, and the keyframe and every video sample until
// the audio's position were discarded as "received too late".
// Without a sync track (audio-only formats) the newest next sample stays the reference.
func nextSegmentStartingPos(tracks []*formatFMP4Track, sync *formatFMP4Track) (time.Time, time.Duration) {
	var refDTS time.Duration
	var refNTP time.Time
	var refReceivedAt time.Time
	var refTrack *formatFMP4Track

	if sync != nil && sync.nextSample != nil {
		refDTS = timestampToDuration(sync.nextSample.dts, int(sync.initTrack.TimeScale))
		refNTP = sync.nextSample.ntp
		refReceivedAt = sync.nextSampleReceivedAt
		refTrack = sync
	} else {
		for _, track := range tracks {
			if track.nextSample != nil && !track.nextSampleWritten {
				dts := timestampToDuration(track.nextSample.dts, int(track.initTrack.TimeScale))
				if dts > refDTS {
					refDTS = dts
					refNTP = track.nextSample.ntp
					refReceivedAt = track.nextSampleReceivedAt
					refTrack = track
				}
			}
		}
	}

	oldestNTP := refNTP
	oldestDTS := refDTS

	for _, track := range tracks {
		if track.nextSample != nil && track.nextSampleWritten && !refNTP.IsZero() && !track.nextSample.ntp.IsZero() {
			// The last PCM packet was committed before a source pause. Keep its
			// established media/absolute-time offset for a resumed packet even
			// when video starts the next segment before audio resumes. Advance
			// the reservation with absolute time, so a stalled track cannot pin it.
			if _, ok := track.initTrack.Codec.(*mcodecs.LPCM); ok {
				elapsed := refNTP.Sub(track.nextSample.ntp)
				if track.f.localNTP() {
					if refReceivedAt.IsZero() || track.nextSampleReceivedAt.IsZero() {
						continue
					}
					elapsed = refReceivedAt.Sub(track.nextSampleReceivedAt)
					if sync != nil && sync.startInitialized && !sync.lastReceivedAt.IsZero() {
						// A shared delivery hiatus need not advance either media clock.
						// Use the reference's observed media step and its previous ingress
						// relation to the held audio, rather than extrapolating the hiatus.
						elapsed = max(time.Duration(0), refDTS-sync.lastDTS+
							sync.lastReceivedAt.Sub(track.nextSampleReceivedAt))
					}
					for _, reference := range track.projectionReferences {
						if reference.track == refTrack {
							// Keep the media/ingress pair that existed when audio was held;
							// later video packets must not reintroduce a shared delivery gap.
							elapsed = max(time.Duration(0), refDTS-reference.dts+
								reference.receivedAt.Sub(track.nextSampleReceivedAt))
							break
						}
					}
				}
				projected := timestampToDuration(track.nextSample.dts, int(track.initTrack.TimeScale)) + elapsed
				offset := refDTS - projected
				if elapsed >= 0 && offset >= -maxBasetime && offset <= ntpDriftTolerance {
					// Reserve the existing one-second interleave window too: the
					// resumed packet can arrive after video with an earlier NTP.
					reserved := max(projected-maxBasetime, refDTS-ntpDriftTolerance)
					if reserved < oldestDTS {
						oldestDTS, oldestNTP = reserved, refNTP.Add(reserved-projected)
					}
				}
			}
		}
		if track.nextSample != nil && !track.nextSampleWritten {
			dts := timestampToDuration(track.nextSample.dts, int(track.initTrack.TimeScale))
			// Fresh audio can have a constant DTS offset from video after RTP
			// track initialization or clock correction. Retain its pending sample
			// without changing its timestamp. The absolute-time bound prevents a
			// stalled audio track from pinning every subsequent segment start.
			reference, pending := refNTP, track.nextSample.ntp
			if track.f.localNTP() {
				reference, pending = refReceivedAt, track.nextSampleReceivedAt
			}
			freshAudio := !track.initTrack.Codec.IsVideo() &&
				!refNTP.IsZero() && !track.nextSample.ntp.IsZero() &&
				!reference.IsZero() && !pending.IsZero() && pending.Sub(reference).Abs() <= maxBasetime
			if ((refDTS-dts) <= maxBasetime || freshAudio) && (dts <= oldestDTS) {
				oldestNTP = track.nextSample.ntp
				oldestDTS = dts
			}
		}
	}

	return oldestNTP, oldestDTS
}

type formatFMP4ProjectionReference struct {
	track      *formatFMP4Track
	dts        time.Duration
	receivedAt time.Time
}

type formatFMP4Track struct {
	f         *formatFMP4
	id        int
	clockRate uint32
	codec     mcodecs.Codec

	initTrack            *fmp4.InitTrack
	nextSample           *formatFMP4Sample
	nextSampleWritten    bool
	nextSampleReceivedAt time.Time
	lastDTS              time.Duration
	lastReceivedAt       time.Time
	projectionReferences []formatFMP4ProjectionReference
	startInitialized     bool
	startDTS             time.Duration
	startNTP             time.Time
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

	alreadyWritten := t.nextSampleWritten
	receivedAt := t.nextSampleReceivedAt
	if t.f.localNTP() {
		t.nextSampleReceivedAt = t.f.receiptNow()
		if _, ok := t.initTrack.Codec.(*mcodecs.LPCM); ok {
			t.projectionReferences = t.projectionReferences[:0]
			for _, reference := range t.f.tracks {
				if reference != t && reference.nextSample != nil && !reference.nextSampleReceivedAt.IsZero() {
					t.projectionReferences = append(t.projectionReferences, formatFMP4ProjectionReference{
						track:      reference,
						dts:        timestampToDuration(reference.nextSample.dts, int(reference.initTrack.TimeScale)),
						receivedAt: reference.nextSampleReceivedAt,
					})
				}
			}
		}
	}
	sample, t.nextSample = t.nextSample, sample
	t.nextSampleWritten = false
	if sample == nil || alreadyWritten {
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
			advance := timestampToDuration(t.nextSample.dts-sample.dts, int(t.initTrack.TimeScale))
			if advance <= 0 || !t.f.localCorrection(dts-t.lastDTS, receivedAt.Sub(t.lastReceivedAt)) {
				return fmt.Errorf("detected drift between recording duration and absolute time, resetting")
			}
			t.startDTS, t.startNTP = dts, sample.ntp
			t.f.rotateSegment = true
		}
	}

	t.lastDTS = dts
	t.lastReceivedAt = receivedAt
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
			advance := timestampToDuration(duration, int(t.initTrack.TimeScale))
			if !t.f.localCorrection(advance, t.nextSampleReceivedAt.Sub(receivedAt)) {
				duration = 0
			}
		}
	}

	sample.Duration = uint32(duration)

	if t.f.currentSegment == nil {
		configuredVideo := false
		for _, track := range t.f.tracks {
			if track.initTrack.Codec.IsVideo() {
				configuredVideo = true
				break
			}
		}
		if configuredVideo && !t.initTrack.Codec.IsVideo() {
			// Charge metadata too, so zero-length samples cannot grow the queue forever.
			size := uint64(len(sample.Payload)) + 128
			limit := uint64(t.f.ri.maxPartSize)
			if size > limit || t.f.initialSize > limit-size {
				t.f.initialSamples = nil
				t.f.initialSize = 0
				return fmt.Errorf("reached maximum startup pre-roll size, resetting")
			}
			t.f.initialSamples = append(t.f.initialSamples, formatFMP4InitialSample{track: t, sample: sample, dts: dts})
			t.f.initialSize += size
			return nil
		}
		if configuredVideo && sample.IsNonSyncSample {
			return nil // first recorded video must be a sync sample
		}
		startDTS, startNTP := dts, sample.ntp
		for _, pending := range t.f.initialSamples {
			if !pending.sample.ntp.IsZero() && pending.dts < startDTS {
				startDTS, startNTP = pending.dts, pending.sample.ntp
			}
		}
		// The first audio unit can still be held as nextSample when video arrives first.
		// Reserve its unchanged timestamp even though its duration is not known yet.
		for _, track := range t.f.tracks {
			if configuredVideo && !track.initTrack.Codec.IsVideo() &&
				track.nextSample != nil && !track.nextSample.ntp.IsZero() {
				pendingDTS := timestampToDuration(track.nextSample.dts, int(track.initTrack.TimeScale))
				if pendingDTS < startDTS {
					startDTS, startNTP = pendingDTS, track.nextSample.ntp
				}
			}
		}
		t.f.streamStartDTS, t.f.streamStartNTP = startDTS, startNTP
		t.f.currentSegment = &formatFMP4Segment{
			f:           t.f,
			startDTS:    startDTS,
			rotationDTS: dts,
			startNTP:    startNTP,
			number:      t.f.nextSegmentNumber,
		}
		t.f.currentSegment.initialize()
		t.f.nextSegmentNumber++
		pending := t.f.initialSamples
		t.f.initialSamples = nil
		t.f.initialSize = 0
		for _, initial := range pending {
			if initial.dts < startDTS {
				t.f.ri.Log(logger.Warn,
					"sample of track %d has unknown absolute time and precedes startup, discarding", initial.track.id)
				continue
			}
			if err := t.f.currentSegment.write(initial.track, initial.sample, initial.dts); err != nil {
				return err
			}
		}
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
		(t.f.rotateSegment || (nextDTS-t.f.currentSegment.rotationDTS) >= t.f.ri.segmentDuration) {
		// A source can stop audio across several video rotations. Its last
		// received LPCM sample has a duration known from its payload; commit it
		// to the old segment before closing, rather than discard it on resume.
		for _, track := range t.f.tracks {
			if track.nextSample != nil {
				reference, pending := t.nextSample.ntp, track.nextSample.ntp
				if t.f.localNTP() {
					reference, pending = t.nextSampleReceivedAt, track.nextSampleReceivedAt
				}
				if !reference.IsZero() && !pending.IsZero() && reference.Sub(pending) > maxBasetime {
					if flushErr := track.flushPendingLPCM(); flushErr != nil {
						return flushErr
					}
				}
			}
		}
		err = t.f.currentSegment.close()
		if err != nil {
			return err
		}

		// a video track switches the segment on its next sample, a sync sample: keep it in the new segment.
		var sync *formatFMP4Track
		if t.initTrack.Codec.IsVideo() {
			sync = t
		}
		oldestNTP, oldestDTS := nextSegmentStartingPos(t.f.tracks, sync)
		if t.f.localNTP() {
			drift := oldestNTP.Sub(t.f.streamStartNTP) - (oldestDTS - t.f.streamStartDTS)
			if drift.Abs() > ntpDriftTolerance {
				t.f.ri.streamID = uuid.New()
				t.f.streamStartDTS, t.f.streamStartNTP = oldestDTS, oldestNTP
			}
		}
		t.f.rotateSegment = false

		t.f.currentSegment = &formatFMP4Segment{
			f:           t.f,
			startDTS:    oldestDTS,
			rotationDTS: nextDTS,
			startNTP:    oldestNTP,
			number:      t.f.nextSegmentNumber,
		}
		t.f.currentSegment.initialize()
		t.f.nextSegmentNumber++
	}

	return nil
}

func (t *formatFMP4Track) flushPendingLPCM() error {
	codec, ok := t.initTrack.Codec.(*mcodecs.LPCM)
	if !ok || t.nextSampleWritten || t.nextSample == nil {
		return nil
	}
	bitsPerSample := codec.BitDepth * codec.ChannelCount
	if bitsPerSample <= 0 || len(t.nextSample.Payload)*8%bitsPerSample != 0 {
		return nil
	}
	dts := timestampToDuration(t.nextSample.dts, int(t.initTrack.TimeScale))
	if dts < t.f.currentSegment.startDTS {
		return nil
	}
	if t.nextSample.ntp.IsZero() {
		return nil
	}
	if t.startInitialized {
		drift := t.nextSample.ntp.Sub(t.startNTP) - (dts - t.startDTS)
		if drift.Abs() > ntpDriftTolerance {
			if !t.f.localCorrection(dts-t.lastDTS, t.nextSampleReceivedAt.Sub(t.lastReceivedAt)) {
				return fmt.Errorf("detected drift between recording duration and absolute time, resetting")
			}
			t.startDTS, t.startNTP = dts, t.nextSample.ntp
			t.f.rotateSegment = true
		}
	}
	t.nextSample.Duration = uint32(len(t.nextSample.Payload) * 8 / bitsPerSample)
	if err := t.f.currentSegment.write(t, t.nextSample, dts); err != nil {
		return err
	}
	if !t.startInitialized {
		t.startDTS, t.startNTP, t.startInitialized = dts, t.nextSample.ntp, true
	}
	t.lastDTS = dts
	t.lastReceivedAt = t.nextSampleReceivedAt
	t.nextSampleWritten = true
	return nil
}
