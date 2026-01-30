package stream

import (
	"context"
	"sync"
	"time"
)

func multiplyAndDivide2(v, m, d time.Duration) time.Duration {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

type offlineSubStream struct {
	stream *Stream

	subStream *SubStream
	ctx       context.Context
	ctxCancel func()
	wg        sync.WaitGroup
	tracks    []*offlineSubStreamTrack
}

func (o *offlineSubStream) initialize() error {
	o.subStream = &SubStream{
		Stream:        o.stream,
		CurDesc:       o.stream.Desc,
		UseRTPPackets: false,
	}
	err := o.subStream.Initialize()
	if err != nil {
		return err
	}

	o.ctx, o.ctxCancel = context.WithCancel(context.Background())

	pos := 0
	o.tracks = make([]*offlineSubStreamTrack, len(o.subStream.CurDesc.Medias))

	for _, media := range o.subStream.CurDesc.Medias {
		for _, forma := range media.Formats {
			t := &offlineSubStreamTrack{
				wg:        &o.wg,
				file:      o.stream.AlwaysAvailableFile,
				pos:       pos,
				ctx:       o.ctx,
				subStream: o.subStream,
				media:     media,
				format:    forma,
			}
			t.initialize()
			o.tracks[pos] = t
			pos++
		}
	}

	return nil
}

func (o *offlineSubStream) close(waitLastSample bool) {
	for _, track := range o.tracks {
		track.waitLastSample = waitLastSample
	}

	o.ctxCancel()
	o.wg.Wait()
}
