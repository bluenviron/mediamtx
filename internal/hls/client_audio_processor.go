package hls

import (
	"context"
	"fmt"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/aac"
	"github.com/aler9/gortsplib/pkg/rtpaac"
	"github.com/pion/rtp"
)

type clientAudioProcessorData struct {
	data []byte
	pts  time.Duration
}

type clientAudioProcessor struct {
	ctx      context.Context
	onTrack  func(gortsplib.Track) error
	onPacket func(*rtp.Packet)

	queue         chan clientAudioProcessorData
	encoder       *rtpaac.Encoder
	clockStartRTC time.Time
}

func newClientAudioProcessor(
	ctx context.Context,
	onTrack func(gortsplib.Track) error,
	onPacket func(*rtp.Packet),
) *clientAudioProcessor {
	p := &clientAudioProcessor{
		ctx:      ctx,
		onTrack:  onTrack,
		onPacket: onPacket,
		queue:    make(chan clientAudioProcessorData, clientQueueSize),
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
	pts time.Duration) error {
	adtsPkts, err := aac.DecodeADTS(data)
	if err != nil {
		return err
	}

	aus := make([][]byte, 0, len(adtsPkts))

	pktPts := pts

	now := time.Now()

	for _, pkt := range adtsPkts {
		elapsed := now.Sub(p.clockStartRTC)

		if pktPts > elapsed {
			select {
			case <-p.ctx.Done():
				return fmt.Errorf("terminated")
			case <-time.After(pktPts - elapsed):
			}
		}

		if p.encoder == nil {
			track, err := gortsplib.NewTrackAAC(97, pkt.Type, pkt.SampleRate, pkt.ChannelCount, nil)
			if err != nil {
				return err
			}

			p.encoder = rtpaac.NewEncoder(97, track.ClockRate(), nil, nil, nil)

			err = p.onTrack(track)
			if err != nil {
				return err
			}
		}

		aus = append(aus, pkt.AU)
		pktPts += 1000 * time.Second / time.Duration(pkt.SampleRate)
	}

	pkts, err := p.encoder.Encode(aus, pts)
	if err != nil {
		return fmt.Errorf("error while encoding AAC: %v", err)
	}

	for _, pkt := range pkts {
		p.onPacket(pkt)
	}

	return nil
}

func (p *clientAudioProcessor) process(
	data []byte,
	pts time.Duration) {
	select {
	case p.queue <- clientAudioProcessorData{data, pts}:
	case <-p.ctx.Done():
	}
}
