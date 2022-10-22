package hls

import (
	"context"
	"fmt"
	"time"
)

type clientProcessorMPEGTSTrackEntry struct {
	data []byte
	dts  time.Duration
	pts  time.Duration
}

type clientProcessorMPEGTSTrack struct {
	startRTC time.Time
	onEntry  func(e *clientProcessorMPEGTSTrackEntry) error

	queue chan *clientProcessorMPEGTSTrackEntry
}

func newClientProcessorMPEGTSTrack(
	startRTC time.Time,
	onEntry func(e *clientProcessorMPEGTSTrackEntry) error,
) *clientProcessorMPEGTSTrack {
	return &clientProcessorMPEGTSTrack{
		startRTC: startRTC,
		onEntry:  onEntry,
		queue:    make(chan *clientProcessorMPEGTSTrackEntry, clientMPEGTSEntryQueueSize),
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

func (t *clientProcessorMPEGTSTrack) processEntry(ctx context.Context, entry *clientProcessorMPEGTSTrackEntry) error {
	elapsed := time.Since(t.startRTC)
	if entry.dts > elapsed {
		select {
		case <-ctx.Done():
			return fmt.Errorf("terminated")
		case <-time.After(entry.dts - elapsed):
		}
	}

	return t.onEntry(entry)
}
