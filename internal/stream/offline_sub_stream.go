package stream

import (
	"context"
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

	for _, media := range o.subStream.CurDesc.Medias {
		for _, forma := range media.Formats {
			t := &offlineSubStreamTrack{
				ctx:       o.ctx,
				subStream: o.subStream,
				media:     media,
				format:    forma,
			}
			t.initialize()
		}
	}

	return nil
}

func (o *offlineSubStream) close() {
	o.ctxCancel()
}
