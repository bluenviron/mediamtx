package hls

import (
	"context"
	"time"

	"github.com/aler9/rtsp-simple-server/internal/hls/fmp4"
)

type clientProcessorFMP4Track struct {
	timeScale            uint32
	ts                   *clientTimeSyncFMP4
	onPartTrackProcessed func(context.Context)
	onEntry              func(time.Duration, []byte) error

	// in
	queue chan *fmp4.PartTrack
}

func newClientProcessorFMP4Track(
	timeScale uint32,
	ts *clientTimeSyncFMP4,
	onPartTrackProcessed func(context.Context),
	onEntry func(time.Duration, []byte) error,
) *clientProcessorFMP4Track {
	return &clientProcessorFMP4Track{
		timeScale:            timeScale,
		ts:                   ts,
		onPartTrackProcessed: onPartTrackProcessed,
		onEntry:              onEntry,
		queue:                make(chan *fmp4.PartTrack, clientFMP4MaxPartTracksPerSegment),
	}
}

func (t *clientProcessorFMP4Track) run(ctx context.Context) error {
	for {
		select {
		case entry := <-t.queue:
			err := t.processPartTrack(ctx, entry)
			if err != nil {
				return err
			}

			t.onPartTrackProcessed(ctx)

		case <-ctx.Done():
			return nil
		}
	}
}

func (t *clientProcessorFMP4Track) processPartTrack(ctx context.Context, pt *fmp4.PartTrack) error {
	rawDTS := pt.BaseTime

	for _, sample := range pt.Samples {
		pts, err := t.ts.convertAndSync(ctx, t.timeScale, rawDTS, sample.PTSOffset)
		if err != nil {
			return err
		}

		if pts >= 0 { // silently discard packets prior to the first packet of the leading track
			err = t.onEntry(pts, sample.Payload)
			if err != nil {
				return err
			}
		}

		rawDTS += uint64(sample.Duration)
	}

	return nil
}
