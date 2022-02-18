package hls

import (
	"context"
	"fmt"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
	"github.com/aler9/gortsplib/pkg/rtph264"
	"github.com/pion/rtp"
)

type clientVideoProcessorData struct {
	data []byte
	pts  time.Duration
	dts  time.Duration
}

type clientVideoProcessor struct {
	ctx      context.Context
	onTrack  func(gortsplib.Track) error
	onPacket func(*rtp.Packet)

	queue         chan clientVideoProcessorData
	sps           []byte
	pps           []byte
	encoder       *rtph264.Encoder
	clockStartRTC time.Time
}

func newClientVideoProcessor(
	ctx context.Context,
	onTrack func(gortsplib.Track) error,
	onPacket func(*rtp.Packet),
) *clientVideoProcessor {
	p := &clientVideoProcessor{
		ctx:      ctx,
		onTrack:  onTrack,
		onPacket: onPacket,
		queue:    make(chan clientVideoProcessorData, clientQueueSize),
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
		return err
	}

	outNALUs := make([][]byte, 0, len(nalus))

	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case h264.NALUTypeSPS:
			if p.sps == nil {
				p.sps = append([]byte(nil), nalu...)

				if p.encoder == nil && p.pps != nil {
					err := p.initializeEncoder()
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

				if p.encoder == nil && p.sps != nil {
					err := p.initializeEncoder()
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

	if p.encoder == nil {
		return nil
	}

	pkts, err := p.encoder.Encode(outNALUs, pts)
	if err != nil {
		return fmt.Errorf("error while encoding H264: %v", err)
	}

	for _, pkt := range pkts {
		p.onPacket(pkt)
	}

	return nil
}

func (p *clientVideoProcessor) process(
	data []byte,
	pts time.Duration,
	dts time.Duration) {
	p.queue <- clientVideoProcessorData{data, pts, dts}
}

func (p *clientVideoProcessor) initializeEncoder() error {
	track, err := gortsplib.NewTrackH264(96, p.sps, p.pps, nil)
	if err != nil {
		return err
	}

	p.encoder = rtph264.NewEncoder(96, nil, nil, nil)

	return p.onTrack(track)
}
