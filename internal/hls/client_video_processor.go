package hls

import (
	"context"
	"fmt"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"

	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type clientVideoProcessorData struct {
	data []byte
	pts  time.Duration
	dts  time.Duration
}

type clientVideoProcessor struct {
	ctx     context.Context
	onTrack func(*gortsplib.TrackH264) error
	onData  func(time.Duration, [][]byte)
	logger  ClientLogger

	queue         chan clientVideoProcessorData
	clockStartRTC time.Time
}

func newClientVideoProcessor(
	ctx context.Context,
	onTrack func(*gortsplib.TrackH264) error,
	onData func(time.Duration, [][]byte),
	logger ClientLogger,
) *clientVideoProcessor {
	p := &clientVideoProcessor{
		ctx:     ctx,
		onTrack: onTrack,
		onData:  onData,
		logger:  logger,
		queue:   make(chan clientVideoProcessorData, clientQueueSize),
	}

	return p
}

func (p *clientVideoProcessor) run() error {
	track := &gortsplib.TrackH264{
		PayloadType: 96,
	}

	err := p.onTrack(track)
	if err != nil {
		return err
	}

	for {
		select {
		case item := <-p.queue:
			err := p.doProcess(item.data, item.pts, item.dts)
			if err != nil {
				return err
			}

		case <-p.ctx.Done():
			return nil
		}
	}
}

func (p *clientVideoProcessor) doProcess(
	data []byte,
	pts time.Duration,
	dts time.Duration,
) error {
	elapsed := time.Since(p.clockStartRTC)
	if dts > elapsed {
		select {
		case <-p.ctx.Done():
			return fmt.Errorf("terminated")
		case <-time.After(dts - elapsed):
		}
	}

	nalus, err := h264.AnnexBUnmarshal(data)
	if err != nil {
		p.logger.Log(logger.Warn, "unable to unmarshal Annex-B: %s", err)
		return nil
	}

	p.onData(pts, nalus)
	return nil
}

func (p *clientVideoProcessor) process(
	data []byte,
	pts time.Duration,
	dts time.Duration,
) {
	select {
	case p.queue <- clientVideoProcessorData{data, pts, dts}:
	case <-p.ctx.Done():
	}
}
