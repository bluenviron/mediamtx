package hls

import (
	"context"
	"fmt"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/mpeg4audio"
)

type clientAudioProcessorData struct {
	data []byte
	pts  time.Duration
}

type clientAudioProcessor struct {
	ctx     context.Context
	onTrack func(*gortsplib.TrackMPEG4Audio) error
	onData  func(time.Duration, []byte)

	trackInitialized bool
	queue            chan clientAudioProcessorData
	clockStartRTC    time.Time
}

func newClientAudioProcessor(
	ctx context.Context,
	onTrack func(*gortsplib.TrackMPEG4Audio) error,
	onData func(time.Duration, []byte),
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
	elapsed := time.Since(p.clockStartRTC)
	if pts > elapsed {
		select {
		case <-p.ctx.Done():
			return fmt.Errorf("terminated")
		case <-time.After(pts - elapsed):
		}
	}

	var adtsPkts mpeg4audio.ADTSPackets
	err := adtsPkts.Unmarshal(data)
	if err != nil {
		return err
	}

	for i, pkt := range adtsPkts {
		if !p.trackInitialized {
			p.trackInitialized = true

			track := &gortsplib.TrackMPEG4Audio{
				PayloadType: 96,
				Config: &mpeg4audio.Config{
					Type:         pkt.Type,
					SampleRate:   pkt.SampleRate,
					ChannelCount: pkt.ChannelCount,
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			}

			err = p.onTrack(track)
			if err != nil {
				return err
			}
		}

		p.onData(
			pts+time.Duration(i)*mpeg4audio.SamplesPerAccessUnit*time.Second/time.Duration(pkt.SampleRate),
			pkt.AU)
	}

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
