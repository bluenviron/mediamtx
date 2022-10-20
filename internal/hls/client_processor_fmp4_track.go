package hls

import (
	"context"
	"fmt"
	"time"

	"github.com/aler9/rtsp-simple-server/internal/hls/fmp4"
)

type clientProcessorFMP4Track struct {
	timeScale            uint32
	startRTC             time.Time
	onPartTrackProcessed func(context.Context)
	onEntry              func(time.Duration, []byte) error

	// in
	queue chan *fmp4.PartTrack
}

func newClientProcessorFMP4Track(
	timeScale uint32,
	startRTC time.Time,
	onPartTrackProcessed func(context.Context),
	onEntry func(time.Duration, []byte) error,
) *clientProcessorFMP4Track {
	return &clientProcessorFMP4Track{
		timeScale:            timeScale,
		startRTC:             startRTC,
		onPartTrackProcessed: onPartTrackProcessed,
		onEntry:              onEntry,
		queue:                make(chan *fmp4.PartTrack, clientSubpartQueueSize),
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
		pts := (time.Duration(sample.PTSOffset) +
			time.Duration(rawDTS)) * time.Second / time.Duration(t.timeScale)
		dts := time.Duration(rawDTS) * time.Second / time.Duration(t.timeScale)

		elapsed := time.Since(t.startRTC)
		if dts > elapsed {
			select {
			case <-ctx.Done():
				return fmt.Errorf("terminated")
			case <-time.After(dts - elapsed):
			}
		}

		err := t.onEntry(pts, sample.Payload)
		if err != nil {
			return err
		}

		rawDTS += uint64(sample.Duration)
	}

	return nil
}
