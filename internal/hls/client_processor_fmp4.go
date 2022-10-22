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

func fmp4PickPrimaryTrack(init *fmp4.Init) int {
	// pick first video track
	for _, track := range init.Tracks {
		if _, ok := track.Track.(*gortsplib.TrackH264); ok {
			return track.ID
		}
	}

	// otherwise, pick first track
	return init.Tracks[0].ID
}

type clientProcessorFMP4 struct {
	segmentQueue *clientSegmentQueue
	logger       ClientLogger
	rp           *clientRoutinePool
	onVideoData  func(time.Duration, [][]byte)
	onAudioData  func(time.Duration, []byte)

	init           fmp4.Init
	primaryTrackID int
	trackProcs     map[int]*clientProcessorFMP4Track
	startBaseTime  uint64

	// in
	subpartProcessed chan struct{}
}

func newClientProcessorFMP4(
	ctx context.Context,
	initFile []byte,
	segmentQueue *clientSegmentQueue,
	logger ClientLogger,
	rp *clientRoutinePool,
	onStreamTracks func(context.Context, []gortsplib.Track) bool,
	onVideoData func(time.Duration, [][]byte),
	onAudioData func(time.Duration, []byte),
) (*clientProcessorFMP4, error) {
	p := &clientProcessorFMP4{
		segmentQueue:     segmentQueue,
		logger:           logger,
		rp:               rp,
		onVideoData:      onVideoData,
		onAudioData:      onAudioData,
		subpartProcessed: make(chan struct{}, clientFMP4MaxPartTracksPerSegment),
	}

	err := p.init.Unmarshal(initFile)
	if err != nil {
		return nil, err
	}

	p.primaryTrackID = fmp4PickPrimaryTrack(&p.init)

	tracks := make([]gortsplib.Track, len(p.init.Tracks))
	for i, track := range p.init.Tracks {
		tracks[i] = track.Track
	}

	ok := onStreamTracks(ctx, tracks)
	if !ok {
		return nil, fmt.Errorf("terminated")
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
			if p.trackProcs == nil {
				if track.ID != p.primaryTrackID {
					continue
				}
				p.initializeTrackProcs(track.BaseTime)
			}

			track.BaseTime -= p.startBaseTime

			proc, ok := p.trackProcs[track.ID]
			if !ok {
				return fmt.Errorf("track ID %d not present in init file", track.ID)
			}

			if processingCount >= (clientFMP4MaxPartTracksPerSegment - 1) {
				return fmt.Errorf("too many part tracks at once")
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

func (p *clientProcessorFMP4) initializeTrackProcs(baseTime uint64) {
	p.startBaseTime = baseTime
	startRTC := time.Now()

	p.trackProcs = make(map[int]*clientProcessorFMP4Track)

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
