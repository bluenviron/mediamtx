package hls

import (
	"context"
	"fmt"
	"time"
)

type clientProcessorMPEGTSTrackEntry interface {
	DTS() time.Duration
}

type clientProcessorMPEGTSTrackEntryVideo struct {
	data []byte
	pts  time.Duration
	dts  time.Duration
}

func (e clientProcessorMPEGTSTrackEntryVideo) DTS() time.Duration {
	return e.dts
}

type clientProcessorMPEGTSTrackEntryAudio struct {
	data []byte
	pts  time.Duration
}

func (e clientProcessorMPEGTSTrackEntryAudio) DTS() time.Duration {
	return e.pts
}

type clientProcessorMPEGTSTrack struct {
	clockStartRTC time.Time
	onEntry       func(e clientProcessorMPEGTSTrackEntry) error

	queue chan clientProcessorMPEGTSTrackEntry
}

func newClientProcessorMPEGTSTrack(
	clockStartRTC time.Time,
	onEntry func(e clientProcessorMPEGTSTrackEntry) error,
) *clientProcessorMPEGTSTrack {
	return &clientProcessorMPEGTSTrack{
		clockStartRTC: clockStartRTC,
		onEntry:       onEntry,
		queue:         make(chan clientProcessorMPEGTSTrackEntry, clientQueueSize),
	}
}

func (t *clientProcessorMPEGTSTrack) run(ctx context.Context) error {
	for {
		select {
		case entry := <-t.queue:
			err := t.processEntry(ctx, entry)
			if err != nil {
				return err
			}

		case <-ctx.Done():
			return nil
		}
	}
}

func (t *clientProcessorMPEGTSTrack) processEntry(ctx context.Context, entry clientProcessorMPEGTSTrackEntry) error {
	elapsed := time.Since(t.clockStartRTC)
	if entry.DTS() > elapsed {
		select {
		case <-ctx.Done():
			return fmt.Errorf("terminated")
		case <-time.After(entry.DTS() - elapsed):
		}
	}

	return t.onEntry(entry)
}

func (t *clientProcessorMPEGTSTrack) push(ctx context.Context, entry clientProcessorMPEGTSTrackEntry) {
	select {
	case t.queue <- entry:
	case <-ctx.Done():
	}
}
