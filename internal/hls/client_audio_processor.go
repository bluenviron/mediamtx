package hls

import (
	"context"
	"fmt"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/aac"
)

type clientAudioProcessorData struct {
	data []byte
	pts  time.Duration
}

type clientAudioProcessor struct {
	ctx     context.Context
	onTrack func(*gortsplib.TrackAAC) error
	onData  func(time.Duration, [][]byte)

	trackInitialized bool
	queue            chan clientAudioProcessorData
	clockStartRTC    time.Time
}

func newClientAudioProcessor(
	ctx context.Context,
	onTrack func(*gortsplib.TrackAAC) error,
	onData func(time.Duration, [][]byte),
) *clientAudioProcessor {
	p := &clientAudioProcessor{
		ctx:     ctx,
		onTrack: onTrack,
		onData:  onData,
		queue:   make(chan clientAudioProcessorData, clientQueueSize),
	}

	return p
}

func (p *clientAudioProcessor) run() error {
	for {
		select {
		case item := <-p.queue:
			err := p.doProcess(item.data, item.pts)
			if err != nil {
				return err
			}

		case <-p.ctx.Done():
			return nil
		}
	}
}

func (p *clientAudioProcessor) doProcess(
	data []byte,
	pts time.Duration,
) error {
	adtsPkts, err := aac.DecodeADTS(data)
	if err != nil {
		return err
	}

	aus := make([][]byte, 0, len(adtsPkts))

	elapsed := time.Since(p.clockStartRTC)
	if pts > elapsed {
		select {
		case <-p.ctx.Done():
			return fmt.Errorf("terminated")
		case <-time.After(pts - elapsed):
		}
	}

	for _, pkt := range adtsPkts {
		if !p.trackInitialized {
			p.trackInitialized = true

			track, err := gortsplib.NewTrackAAC(96, pkt.Type, pkt.SampleRate, pkt.ChannelCount, nil, 13, 3, 3)
			if err != nil {
				return err
			}

			err = p.onTrack(track)
			if err != nil {
				return err
			}
		}

		aus = append(aus, pkt.AU)
	}

	p.onData(pts, aus)
	return nil
}

func (p *clientAudioProcessor) process(
	data []byte,
	pts time.Duration,
) {
	select {
	case p.queue <- clientAudioProcessorData{data, pts}:
	case <-p.ctx.Done():
	}
}
