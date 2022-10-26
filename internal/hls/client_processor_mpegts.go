package hls

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
	"github.com/aler9/gortsplib/pkg/mpeg4audio"
	"github.com/asticode/go-astits"

	"github.com/aler9/rtsp-simple-server/internal/hls/mpegts"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

func mpegtsPickLeadingTrack(mpegtsTracks []*mpegts.Track) uint16 {
	// pick first video track
	for _, mt := range mpegtsTracks {
		if _, ok := mt.Track.(*gortsplib.TrackH264); ok {
			return mt.ES.ElementaryPID
		}
	}

	// otherwise, pick first track
	return mpegtsTracks[0].ES.ElementaryPID
}

type clientProcessorMPEGTS struct {
	isLeading            bool
	segmentQueue         *clientSegmentQueue
	logger               ClientLogger
	rp                   *clientRoutinePool
	onStreamTracks       func(context.Context, []gortsplib.Track) bool
	onSetLeadingTimeSync func(clientTimeSync)
	onGetLeadingTimeSync func(context.Context) (clientTimeSync, bool)
	onVideoData          func(time.Duration, [][]byte)
	onAudioData          func(time.Duration, []byte)

	mpegtsTracks    []*mpegts.Track
	leadingTrackPID uint16
	trackProcs      map[uint16]*clientProcessorMPEGTSTrack
}

func newClientProcessorMPEGTS(
	isLeading bool,
	segmentQueue *clientSegmentQueue,
	logger ClientLogger,
	rp *clientRoutinePool,
	onStreamTracks func(context.Context, []gortsplib.Track) bool,
	onSetLeadingTimeSync func(clientTimeSync),
	onGetLeadingTimeSync func(context.Context) (clientTimeSync, bool),
	onVideoData func(time.Duration, [][]byte),
	onAudioData func(time.Duration, []byte),
) *clientProcessorMPEGTS {
	return &clientProcessorMPEGTS{
		isLeading:            isLeading,
		segmentQueue:         segmentQueue,
		logger:               logger,
		rp:                   rp,
		onStreamTracks:       onStreamTracks,
		onSetLeadingTimeSync: onSetLeadingTimeSync,
		onGetLeadingTimeSync: onGetLeadingTimeSync,
		onVideoData:          onVideoData,
		onAudioData:          onAudioData,
	}
}

func (p *clientProcessorMPEGTS) run(ctx context.Context) error {
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

func (p *clientProcessorMPEGTS) processSegment(ctx context.Context, byts []byte) error {
	if p.mpegtsTracks == nil {
		var err error
		p.mpegtsTracks, err = mpegts.FindTracks(byts)
		if err != nil {
			return err
		}

		p.leadingTrackPID = mpegtsPickLeadingTrack(p.mpegtsTracks)

		tracks := make([]gortsplib.Track, len(p.mpegtsTracks))
		for i, mt := range p.mpegtsTracks {
			tracks[i] = mt.Track
		}

		ok := p.onStreamTracks(ctx, tracks)
		if !ok {
			return fmt.Errorf("terminated")
		}
	}

	dem := astits.NewDemuxer(context.Background(), bytes.NewReader(byts))

	for {
		data, err := dem.NextData()
		if err != nil {
			if err == astits.ErrNoMorePackets {
				return nil
			}
			if strings.HasPrefix(err.Error(), "astits: parsing PES data failed") {
				continue
			}
			return err
		}

		if data.PES == nil {
			continue
		}

		if data.PES.Header.OptionalHeader == nil ||
			data.PES.Header.OptionalHeader.PTSDTSIndicator == astits.PTSDTSIndicatorNoPTSOrDTS ||
			data.PES.Header.OptionalHeader.PTSDTSIndicator == astits.PTSDTSIndicatorIsForbidden {
			return fmt.Errorf("PTS is missing")
		}

		if p.trackProcs == nil {
			var ts *clientTimeSyncMPEGTS

			if p.isLeading {
				if data.PID != p.leadingTrackPID {
					continue
				}

				var dts int64
				if data.PES.Header.OptionalHeader.PTSDTSIndicator == astits.PTSDTSIndicatorBothPresent {
					dts = data.PES.Header.OptionalHeader.DTS.Base
				} else {
					dts = data.PES.Header.OptionalHeader.PTS.Base
				}

				ts = newClientTimeSyncMPEGTS(dts)
				p.onSetLeadingTimeSync(ts)
			} else {
				rawTS, ok := p.onGetLeadingTimeSync(ctx)
				if !ok {
					return fmt.Errorf("terminated")
				}

				ts, ok = rawTS.(*clientTimeSyncMPEGTS)
				if !ok {
					return fmt.Errorf("stream playlists are mixed MPEGTS/FMP4")
				}
			}

			p.initializeTrackProcs(ts)
		}

		proc, ok := p.trackProcs[data.PID]
		if !ok {
			continue
		}

		select {
		case proc.queue <- data.PES:
		case <-ctx.Done():
		}
	}
}

func (p *clientProcessorMPEGTS) initializeTrackProcs(ts *clientTimeSyncMPEGTS) {
	p.trackProcs = make(map[uint16]*clientProcessorMPEGTSTrack)

	for _, mt := range p.mpegtsTracks {
		var cb func(time.Duration, []byte) error

		switch mt.Track.(type) {
		case *gortsplib.TrackH264:
			cb = func(pts time.Duration, payload []byte) error {
				nalus, err := h264.AnnexBUnmarshal(payload)
				if err != nil {
					p.logger.Log(logger.Warn, "unable to decode Annex-B: %s", err)
					return nil
				}

				p.onVideoData(pts, nalus)
				return nil
			}

		case *gortsplib.TrackMPEG4Audio:
			cb = func(pts time.Duration, payload []byte) error {
				var adtsPkts mpeg4audio.ADTSPackets
				err := adtsPkts.Unmarshal(payload)
				if err != nil {
					return fmt.Errorf("unable to decode ADTS: %s", err)
				}

				for i, pkt := range adtsPkts {
					p.onAudioData(
						pts+time.Duration(i)*mpeg4audio.SamplesPerAccessUnit*time.Second/time.Duration(pkt.SampleRate),
						pkt.AU)
				}

				return nil
			}
		}

		proc := newClientProcessorMPEGTSTrack(
			ts,
			cb,
		)
		p.rp.add(proc)
		p.trackProcs[mt.ES.ElementaryPID] = proc
	}
}
