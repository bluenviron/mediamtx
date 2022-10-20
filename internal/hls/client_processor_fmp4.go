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
	videoProc         *clientProcessorFMP4Track
	audioProc         *clientProcessorFMP4Track
	tracksInitialized bool
	startBaseTime     uint64

	// in
	subpartProcessed chan struct{}
}

func newClientProcessorFMP4(
	initFile []byte,
	segmentQueue *clientSegmentQueue,
	logger ClientLogger,
	rp *clientRoutinePool,
	onTracks func(*gortsplib.TrackH264, *gortsplib.TrackMPEG4Audio) error,
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
		subpartProcessed: make(chan struct{}),
	}

	var videoTrack *gortsplib.TrackH264
	if init.VideoTrack != nil {
		videoTrack = init.VideoTrack.Track.(*gortsplib.TrackH264)
	}

	var audioTrack *gortsplib.TrackMPEG4Audio
	if init.AudioTrack != nil {
		audioTrack = init.AudioTrack.Track.(*gortsplib.TrackMPEG4Audio)
	}

	err = onTracks(videoTrack, audioTrack)
	if err != nil {
		return nil, err
	}

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

				if p.init.VideoTrack != nil {
					p.videoProc = newClientProcessorFMP4Track(
						p.init.VideoTrack.TimeScale,
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
					p.rp.add(p.videoProc)
				}

				if p.init.AudioTrack != nil {
					p.audioProc = newClientProcessorFMP4Track(
						p.init.AudioTrack.TimeScale,
						startRTC,
						p.onPartTrackProcessed,
						func(pts time.Duration, payload []byte) error {
							return nil
						},
					)
					p.rp.add(p.audioProc)
				}
			}

			track.BaseTime -= p.startBaseTime

			if p.init.VideoTrack != nil && track.ID == p.init.VideoTrack.ID {
				select {
				case p.videoProc.queue <- track:
				case <-ctx.Done():
					return fmt.Errorf("terminated")
				}

				processingCount++
			}
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
