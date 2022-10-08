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

	"github.com/aler9/rtsp-simple-server/internal/hls/mpegtstimedec"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type clientProcessorMPEGTS struct {
	segmentQueue *clientSegmentQueue
	logger       ClientLogger
	rp           *clientRoutinePool
	onTracks     func(*gortsplib.TrackH264, *gortsplib.TrackMPEG4Audio) error
	onVideoData  func(time.Duration, [][]byte)
	onAudioData  func(time.Duration, []byte)

	tracksParsed     bool
	clockInitialized bool
	timeDec          *mpegtstimedec.Decoder
	startDTS         time.Duration
	videoPID         *uint16
	audioPID         *uint16
	videoTrack       *gortsplib.TrackH264
	audioTrack       *gortsplib.TrackMPEG4Audio
	videoProc        *clientProcessorMPEGTSTrack
	audioProc        *clientProcessorMPEGTSTrack
}

func newClientProcessorMPEGTS(
	segmentQueue *clientSegmentQueue,
	logger ClientLogger,
	rp *clientRoutinePool,
	onTracks func(*gortsplib.TrackH264, *gortsplib.TrackMPEG4Audio) error,
	onVideoData func(time.Duration, [][]byte),
	onAudioData func(time.Duration, []byte),
) *clientProcessorMPEGTS {
	return &clientProcessorMPEGTS{
		segmentQueue: segmentQueue,
		logger:       logger,
		rp:           rp,
		timeDec:      mpegtstimedec.New(),
		onTracks:     onTracks,
		onVideoData:  onVideoData,
		onAudioData:  onAudioData,
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

	if !p.tracksParsed {
		p.tracksParsed = true

		err := p.parseTracks(dem)
		if err != nil {
			return err
		}

		// rewind demuxer in order to read again the audio packet that was used to create the track
		if p.audioTrack != nil {
			dem = astits.NewDemuxer(context.Background(), bytes.NewReader(byts))
		}
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

		if p.videoPID != nil && data.PID == *p.videoPID {
			var dts time.Duration
			if data.PES.Header.OptionalHeader.PTSDTSIndicator == astits.PTSDTSIndicatorBothPresent {
				diff := time.Duration((data.PES.Header.OptionalHeader.PTS.Base-
					data.PES.Header.OptionalHeader.DTS.Base)&0x1FFFFFFFF) *
					time.Second / 90000
				dts = pts - diff
			} else {
				dts = pts
			}

			if !p.clockInitialized {
				p.clockInitialized = true
				p.startDTS = dts
				now := time.Now()
				p.initializeTrackProcs(now)
			}

			pts -= p.startDTS
			dts -= p.startDTS

			p.videoProc.push(ctx, &clientProcessorMPEGTSTrackEntryVideo{
				data: data.PES.Data,
				pts:  pts,
				dts:  dts,
			})
		} else if p.audioPID != nil && data.PID == *p.audioPID {
			if !p.clockInitialized {
				p.clockInitialized = true
				p.startDTS = pts
				now := time.Now()
				p.initializeTrackProcs(now)
			}

			pts -= p.startDTS

			p.audioProc.push(ctx, &clientProcessorMPEGTSTrackEntryAudio{
				data: data.PES.Data,
				pts:  pts,
			})
		}
	}
}

func (p *clientProcessorMPEGTS) parseTracks(dem *astits.Demuxer) error {
	// find and parse PMT
	for {
		data, err := dem.NextData()
		if err != nil {
			return err
		}

		if data.PMT != nil {
			for _, e := range data.PMT.ElementaryStreams {
				switch e.StreamType {
				case astits.StreamTypeH264Video:
					if p.videoPID != nil {
						return fmt.Errorf("multiple video/audio tracks are not supported")
					}

					v := e.ElementaryPID
					p.videoPID = &v

				case astits.StreamTypeAACAudio:
					if p.audioPID != nil {
						return fmt.Errorf("multiple video/audio tracks are not supported")
					}

					v := e.ElementaryPID
					p.audioPID = &v
				}
			}
			break
		}
	}

	if p.videoPID == nil && p.audioPID == nil {
		return fmt.Errorf("stream doesn't contain tracks with supported codecs (H264 or AAC)")
	}

	if p.videoPID != nil {
		p.videoTrack = &gortsplib.TrackH264{
			PayloadType: 96,
		}

		if p.audioPID == nil {
			err := p.onTracks(p.videoTrack, nil)
			if err != nil {
				return err
			}
		}
	}

	// find and parse first audio packet
	if p.audioPID != nil {
		for {
			data, err := dem.NextData()
			if err != nil {
				return err
			}

			if data.PES == nil || data.PID != *p.audioPID {
				continue
			}

			var adtsPkts mpeg4audio.ADTSPackets
			err = adtsPkts.Unmarshal(data.PES.Data)
			if err != nil {
				return fmt.Errorf("unable to decode ADTS: %s", err)
			}

			pkt := adtsPkts[0]
			p.audioTrack = &gortsplib.TrackMPEG4Audio{
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

			err = p.onTracks(p.videoTrack, p.audioTrack)
			if err != nil {
				return err
			}

			break
		}
	}

	return nil
}

func (p *clientProcessorMPEGTS) initializeTrackProcs(clockStartRTC time.Time) {
	if p.videoTrack != nil {
		p.videoProc = newClientProcessorMPEGTSTrack(
			clockStartRTC,
			func(e clientProcessorMPEGTSTrackEntry) error {
				vd := e.(*clientProcessorMPEGTSTrackEntryVideo)

				nalus, err := h264.AnnexBUnmarshal(vd.data)
				if err != nil {
					p.logger.Log(logger.Warn, "unable to decode Annex-B: %s", err)
					return nil
				}

				p.onVideoData(vd.pts, nalus)
				return nil
			},
		)
		p.rp.add(p.videoProc.run)
	}

	if p.audioTrack != nil {
		p.audioProc = newClientProcessorMPEGTSTrack(
			clockStartRTC,
			func(e clientProcessorMPEGTSTrackEntry) error {
				ad := e.(*clientProcessorMPEGTSTrackEntryAudio)

				var adtsPkts mpeg4audio.ADTSPackets
				err := adtsPkts.Unmarshal(ad.data)
				if err != nil {
					return fmt.Errorf("unable to decode ADTS: %s", err)
				}

				for i, pkt := range adtsPkts {
					p.onAudioData(
						ad.pts+time.Duration(i)*mpeg4audio.SamplesPerAccessUnit*time.Second/time.Duration(pkt.SampleRate),
						pkt.AU)
				}

				return nil
			},
		)
		p.rp.add(p.audioProc.run)
	}
}
