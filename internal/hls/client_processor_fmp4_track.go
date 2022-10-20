package hls

import (
	"context"
	"fmt"
	"time"

	"github.com/aler9/rtsp-simple-server/internal/hls/fmp4"
)

type clientProcessorFMP4Track struct {
	timeScale          uint32
	onSubpartProcessed func(context.Context)
	onEntry            func(time.Duration, []byte) error

	queue    chan *fmp4.Subpart
	startRTC time.Time
}

func newClientProcessorFMP4Track(
	timeScale uint32,
	onSubpartProcessed func(context.Context),
	onEntry func(time.Duration, []byte) error,
) *clientProcessorFMP4Track {
	return &clientProcessorFMP4Track{
		timeScale:          timeScale,
		onSubpartProcessed: onSubpartProcessed,
		onEntry:            onEntry,
		queue:              make(chan *fmp4.Subpart, clientSubpartQueueSize),
	}
}

func (t *clientProcessorFMP4Track) run(ctx context.Context) error {
	for {
		select {
		case entry := <-t.queue:
			err := t.processSubpart(ctx, entry)
			if err != nil {
				return err
			}

			t.onSubpartProcessed(ctx)

		case <-ctx.Done():
			return nil
		}
	}
}

func (t *clientProcessorFMP4Track) processSubpart(ctx context.Context, pt *fmp4.Subpart) error {
	rawDTS := pt.BaseTime
	offset := uint64(0)

	for _, entry := range pt.Entries {
		pts := (time.Duration(entry.SampleCompositionTimeOffsetV1) +
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

		err := t.onEntry(pts, pt.Data[offset:offset+uint64(entry.SampleSize)])
		if err != nil {
			return err
		}

		rawDTS += uint64(entry.SampleDuration)
		offset += uint64(entry.SampleSize)
	}

	return nil
}
