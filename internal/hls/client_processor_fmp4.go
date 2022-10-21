package hls

import (
	"context"
	"fmt"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"

	"github.com/aler9/rtsp-simple-server/internal/hls/fmp4"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type clientProcessorFMP4 struct {
	segmentQueue *clientSegmentQueue
	logger       ClientLogger
	rp           *clientRoutinePool
	onVideoData  func(time.Duration, [][]byte)
	onAudioData  func(time.Duration, []byte)

	init              *fmp4.Init
	trackProcs        map[int]*clientProcessorFMP4Track
	tracksInitialized bool
	startBaseTime     uint64

	// in
	subpartProcessed chan struct{}
}

func newClientProcessorFMP4(
	ctx context.Context,
	initFile []byte,
	segmentQueue *clientSegmentQueue,
	logger ClientLogger,
	rp *clientRoutinePool,
	onStreamTracks func(context.Context, []gortsplib.Track),
	onVideoData func(time.Duration, [][]byte),
	onAudioData func(time.Duration, []byte),
) (*clientProcessorFMP4, error) {
	var init fmp4.Init
	err := init.Unmarshal(initFile)
	if err != nil {
		return nil, err
	}

	p := &clientProcessorFMP4{
		segmentQueue:     segmentQueue,
		logger:           logger,
		rp:               rp,
		onVideoData:      onVideoData,
		onAudioData:      onAudioData,
		init:             &init,
		trackProcs:       make(map[int]*clientProcessorFMP4Track),
		subpartProcessed: make(chan struct{}),
	}

	tracks := make([]gortsplib.Track, len(init.Tracks))
	for i, track := range init.Tracks {
		tracks[i] = track.Track
	}

	onStreamTracks(ctx, tracks)

	return p, nil
}

func (p *clientProcessorFMP4) run(ctx context.Context) error {
	for {
		seg, ok := p.segmentQueue.pull(ctx)
		if !ok {
			return fmt.Errorf("terminated")
		}

		err := p.processSegment(ctx, seg)
		if err != nil {
			return err
		}
	}
}

func (p *clientProcessorFMP4) processSegment(ctx context.Context, byts []byte) error {
	p.logger.Log(logger.Debug, "processing segment")

	var parts fmp4.Parts
	err := parts.Unmarshal(byts)
	if err != nil {
		return err
	}

	processingCount := 0

	for _, part := range parts {
		for _, track := range part.Tracks {
			if !p.tracksInitialized {
				p.tracksInitialized = true
				p.startBaseTime = track.BaseTime
				startRTC := time.Now()

				for _, track := range p.init.Tracks {
					var proc *clientProcessorFMP4Track

					switch track.Track.(type) {
					case *gortsplib.TrackH264:
						proc = newClientProcessorFMP4Track(
							track.TimeScale,
							startRTC,
							p.onPartTrackProcessed,
							func(pts time.Duration, payload []byte) error {
								nalus, err := h264.AVCCUnmarshal(payload)
								if err != nil {
									return err
								}

								p.onVideoData(pts, nalus)
								return nil
							},
						)

					case *gortsplib.TrackMPEG4Audio:
						proc = newClientProcessorFMP4Track(
							track.TimeScale,
							startRTC,
							p.onPartTrackProcessed,
							func(pts time.Duration, payload []byte) error {
								p.onAudioData(pts, payload)
								return nil
							},
						)
					}

					p.rp.add(proc)
					p.trackProcs[track.ID] = proc
				}
			}

			track.BaseTime -= p.startBaseTime

			proc, ok := p.trackProcs[track.ID]
			if !ok {
				return fmt.Errorf("track ID %d not present in init file", track.ID)
			}

			select {
			case proc.queue <- track:
			case <-ctx.Done():
				return fmt.Errorf("terminated")
			}
			processingCount++
		}
	}

	for i := 0; i < processingCount; i++ {
		select {
		case <-p.subpartProcessed:
		case <-ctx.Done():
			return fmt.Errorf("terminated")
		}
	}

	return nil
}

func (p *clientProcessorFMP4) onPartTrackProcessed(ctx context.Context) {
	select {
	case p.subpartProcessed <- struct{}{}:
	case <-ctx.Done():
	}
}
