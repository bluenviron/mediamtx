package hls

import (
	"context"
	"time"

	"github.com/asticode/go-astits"
)

type clientProcessorMPEGTSTrack struct {
	ts      *clientTimeSyncMPEGTS
	onEntry func(time.Duration, []byte) error

	queue chan *astits.PESData
}

func newClientProcessorMPEGTSTrack(
	ts *clientTimeSyncMPEGTS,
	onEntry func(time.Duration, []byte) error,
) *clientProcessorMPEGTSTrack {
	return &clientProcessorMPEGTSTrack{
		ts:      ts,
		onEntry: onEntry,
		queue:   make(chan *astits.PESData, clientMPEGTSEntryQueueSize),
	}
}

func (t *clientProcessorMPEGTSTrack) run(ctx context.Context) error {
	for {
		select {
		case pes := <-t.queue:
			err := t.processEntry(ctx, pes)
			if err != nil {
				return err
			}

		case <-ctx.Done():
			return nil
		}
	}
}

func (t *clientProcessorMPEGTSTrack) processEntry(ctx context.Context, pes *astits.PESData) error {
	rawPTS := pes.Header.OptionalHeader.PTS.Base
	var rawDTS int64
	if pes.Header.OptionalHeader.PTSDTSIndicator == astits.PTSDTSIndicatorBothPresent {
		rawDTS = pes.Header.OptionalHeader.DTS.Base
	} else {
		rawDTS = rawPTS
	}

	pts, err := t.ts.convertAndSync(ctx, rawDTS, rawPTS)
	if err != nil {
		return err
	}

	// silently discard packets prior to the first packet of the leading track
	if pts < 0 {
		return nil
	}

	return t.onEntry(pts, pes.Data)
}
