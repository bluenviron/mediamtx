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

type clientProcessorMPEGTS struct {
	segmentQueue   *clientSegmentQueue
	logger         ClientLogger
	rp             *clientRoutinePool
	onStreamTracks func(context.Context, []gortsplib.Track)
	onVideoData    func(time.Duration, [][]byte)
	onAudioData    func(time.Duration, []byte)

	mpegtsTracks []*mpegts.Track
	timeDec      *mpegts.TimeDecoder
	startDTS     time.Duration
	trackProcs   map[uint16]*clientProcessorMPEGTSTrack
}

func newClientProcessorMPEGTS(
	segmentQueue *clientSegmentQueue,
	logger ClientLogger,
	rp *clientRoutinePool,
	onStreamTracks func(context.Context, []gortsplib.Track),
	onVideoData func(time.Duration, [][]byte),
	onAudioData func(time.Duration, []byte),
) *clientProcessorMPEGTS {
	return &clientProcessorMPEGTS{
		segmentQueue:   segmentQueue,
		logger:         logger,
		rp:             rp,
		timeDec:        mpegts.NewTimeDecoder(),
		onStreamTracks: onStreamTracks,
		onVideoData:    onVideoData,
		onAudioData:    onAudioData,
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
	p.logger.Log(logger.Debug, "processing segment")

	dem := astits.NewDemuxer(context.Background(), bytes.NewReader(byts))

	if p.mpegtsTracks == nil {
		var err error
		p.mpegtsTracks, err = mpegts.FindTracks(dem)
		if err != nil {
			return err
		}

		// rewind demuxer in order to read again the audio packet that was used to create the track
		dem = astits.NewDemuxer(context.Background(), bytes.NewReader(byts))

		tracks := make([]gortsplib.Track, len(p.mpegtsTracks))
		for i, mt := range p.mpegtsTracks {
			tracks[i] = mt.Track
		}

		p.onStreamTracks(ctx, tracks)
	}

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

		pts := p.timeDec.Decode(data.PES.Header.OptionalHeader.PTS.Base)

		var dts time.Duration
		if data.PES.Header.OptionalHeader.PTSDTSIndicator == astits.PTSDTSIndicatorBothPresent {
			diff := time.Duration((data.PES.Header.OptionalHeader.PTS.Base-
				data.PES.Header.OptionalHeader.DTS.Base)&0x1FFFFFFFF) *
				time.Second / 90000
			dts = pts - diff
		} else {
			dts = pts
		}

		if p.trackProcs == nil {
			p.initializeTrackProcs(dts)
		}

		pts -= p.startDTS
		dts -= p.startDTS

		proc, ok := p.trackProcs[data.PID]
		if !ok {
			return fmt.Errorf("received data from track not present into PMT (%d)", data.PID)
		}

		proc.push(ctx, &clientProcessorMPEGTSTrackEntry{
			data: data.PES.Data,
			pts:  pts,
			dts:  dts,
		})
	}
}

func (p *clientProcessorMPEGTS) initializeTrackProcs(dts time.Duration) {
	p.startDTS = dts
	startRTC := time.Now()

	p.trackProcs = make(map[uint16]*clientProcessorMPEGTSTrack)

	for _, mt := range p.mpegtsTracks {
		var proc *clientProcessorMPEGTSTrack

		switch mt.Track.(type) {
		case *gortsplib.TrackH264:
			proc = newClientProcessorMPEGTSTrack(
				startRTC,
				func(e *clientProcessorMPEGTSTrackEntry) error {
					nalus, err := h264.AnnexBUnmarshal(e.data)
					if err != nil {
						p.logger.Log(logger.Warn, "unable to decode Annex-B: %s", err)
						return nil
					}

					p.onVideoData(e.pts, nalus)
					return nil
				},
			)

		case *gortsplib.TrackMPEG4Audio:
			proc = newClientProcessorMPEGTSTrack(
				startRTC,
				func(e *clientProcessorMPEGTSTrackEntry) error {
					var adtsPkts mpeg4audio.ADTSPackets
					err := adtsPkts.Unmarshal(e.data)
					if err != nil {
						return fmt.Errorf("unable to decode ADTS: %s", err)
					}

					for i, pkt := range adtsPkts {
						p.onAudioData(
							e.pts+time.Duration(i)*mpeg4audio.SamplesPerAccessUnit*time.Second/time.Duration(pkt.SampleRate),
							pkt.AU)
					}

					return nil
				},
			)
		}

		p.rp.add(proc)
		p.trackProcs[mt.ES.ElementaryPID] = proc
	}
}
