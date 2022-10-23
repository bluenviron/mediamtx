package hls

import (
	"context"
	"fmt"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"

	"github.com/aler9/rtsp-simple-server/internal/hls/fmp4"
)

func fmp4PickLeadingTrack(init *fmp4.Init) int {
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
	isLeading            bool
	segmentQueue         *clientSegmentQueue
	logger               ClientLogger
	rp                   *clientRoutinePool
	onSetLeadingTimeSync func(clientTimeSync)
	onGetLeadingTimeSync func(context.Context) (clientTimeSync, bool)
	onVideoData          func(time.Duration, [][]byte)
	onAudioData          func(time.Duration, []byte)

	init           fmp4.Init
	leadingTrackID int
	trackProcs     map[int]*clientProcessorFMP4Track

	// in
	subpartProcessed chan struct{}
}

func newClientProcessorFMP4(
	ctx context.Context,
	isLeading bool,
	initFile []byte,
	segmentQueue *clientSegmentQueue,
	logger ClientLogger,
	rp *clientRoutinePool,
	onStreamTracks func(context.Context, []gortsplib.Track) bool,
	onSetLeadingTimeSync func(clientTimeSync),
	onGetLeadingTimeSync func(context.Context) (clientTimeSync, bool),
	onVideoData func(time.Duration, [][]byte),
	onAudioData func(time.Duration, []byte),
) (*clientProcessorFMP4, error) {
	p := &clientProcessorFMP4{
		isLeading:            isLeading,
		segmentQueue:         segmentQueue,
		logger:               logger,
		rp:                   rp,
		onSetLeadingTimeSync: onSetLeadingTimeSync,
		onGetLeadingTimeSync: onGetLeadingTimeSync,
		onVideoData:          onVideoData,
		onAudioData:          onAudioData,
		subpartProcessed:     make(chan struct{}, clientFMP4MaxPartTracksPerSegment),
	}

	err := p.init.Unmarshal(initFile)
	if err != nil {
		return nil, err
	}

	p.leadingTrackID = fmp4PickLeadingTrack(&p.init)

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
	var parts fmp4.Parts
	err := parts.Unmarshal(byts)
	if err != nil {
		return err
	}

	processingCount := 0

	for _, part := range parts {
		for _, track := range part.Tracks {
			if p.trackProcs == nil {
				var ts *clientTimeSyncFMP4

				if p.isLeading {
					if track.ID != p.leadingTrackID {
						continue
					}

					timeScale := func() uint32 {
						for _, track := range p.init.Tracks {
							if track.ID == p.leadingTrackID {
								return track.TimeScale
							}
						}
						return 0
					}()
					ts = newClientTimeSyncFMP4(timeScale, track.BaseTime)
					p.onSetLeadingTimeSync(ts)
				} else {
					rawTS, ok := p.onGetLeadingTimeSync(ctx)
					if !ok {
						return fmt.Errorf("terminated")
					}

					ts, ok = rawTS.(*clientTimeSyncFMP4)
					if !ok {
						return fmt.Errorf("stream playlists are mixed MPEGTS/FMP4")
					}
				}

				p.initializeTrackProcs(ts)
			}

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

func (p *clientProcessorFMP4) initializeTrackProcs(ts *clientTimeSyncFMP4) {
	p.trackProcs = make(map[int]*clientProcessorFMP4Track)

	for _, track := range p.init.Tracks {
		var cb func(time.Duration, []byte) error

		switch track.Track.(type) {
		case *gortsplib.TrackH264:
			cb = func(pts time.Duration, payload []byte) error {
				nalus, err := h264.AVCCUnmarshal(payload)
				if err != nil {
					return err
				}

				p.onVideoData(pts, nalus)
				return nil
			}

		case *gortsplib.TrackMPEG4Audio:
			cb = func(pts time.Duration, payload []byte) error {
				p.onAudioData(pts, payload)
				return nil
			}
		}

		proc := newClientProcessorFMP4Track(
			track.TimeScale,
			ts,
			p.onPartTrackProcessed,
			cb,
		)
		p.rp.add(proc)
		p.trackProcs[track.ID] = proc
	}
}
