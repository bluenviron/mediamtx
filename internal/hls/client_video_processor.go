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

	trackInitialized bool
	queue            chan clientVideoProcessorData
	sps              []byte
	pps              []byte
	clockStartRTC    time.Time
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
	dts time.Duration) error {
	elapsed := time.Since(p.clockStartRTC)
	if dts > elapsed {
		select {
		case <-p.ctx.Done():
			return fmt.Errorf("terminated")
		case <-time.After(dts - elapsed):
		}
	}

	nalus, err := h264.DecodeAnnexB(data)
	if err != nil {
		p.logger.Log(logger.Warn, "unable to decode Annex-B: %s", err)
		return nil
	}

	outNALUs := make([][]byte, 0, len(nalus))

	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case h264.NALUTypeSPS:
			if p.sps == nil {
				p.sps = append([]byte(nil), nalu...)

				if !p.trackInitialized && p.pps != nil {
					p.trackInitialized = true
					err := p.initializeTrack()
					if err != nil {
						return err
					}
				}
			}

			// remove since it's not needed
			continue

		case h264.NALUTypePPS:
			if p.pps == nil {
				p.pps = append([]byte(nil), nalu...)

				if !p.trackInitialized && p.sps != nil {
					p.trackInitialized = true
					err := p.initializeTrack()
					if err != nil {
						return err
					}
				}
			}

			// remove since it's not needed
			continue

		case h264.NALUTypeAccessUnitDelimiter:
			// remove since it's not needed
			continue
		}

		outNALUs = append(outNALUs, nalu)
	}

	if len(outNALUs) == 0 {
		return nil
	}

	if !p.trackInitialized {
		return nil
	}

	p.onData(pts, outNALUs)
	return nil
}

func (p *clientVideoProcessor) process(
	data []byte,
	pts time.Duration,
	dts time.Duration) {
	select {
	case p.queue <- clientVideoProcessorData{data, pts, dts}:
	case <-p.ctx.Done():
	}
}

func (p *clientVideoProcessor) initializeTrack() error {
	track, err := gortsplib.NewTrackH264(96, p.sps, p.pps, nil)
	if err != nil {
		return err
	}

	return p.onTrack(track)
}
