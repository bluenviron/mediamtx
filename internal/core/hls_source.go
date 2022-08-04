package core

import (
	"context"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
	"github.com/aler9/gortsplib/pkg/rtpaac"
	"github.com/aler9/gortsplib/pkg/rtph264"

	"github.com/aler9/rtsp-simple-server/internal/hls"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type hlsSourceParent interface {
	log(logger.Level, string, ...interface{})
	onSourceStaticImplSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	onSourceStaticImplSetNotReady(req pathSourceStaticSetNotReadyReq)
}

type hlsSource struct {
	ur          string
	fingerprint string
	parent      hlsSourceParent
}

func newHLSSource(
	ur string,
	fingerprint string,
	parent hlsSourceParent,
) *hlsSource {
	return &hlsSource{
		ur:          ur,
		fingerprint: fingerprint,
		parent:      parent,
	}
}

func (s *hlsSource) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.log(level, "[hls source] "+format, args...)
}

// run implements sourceStaticImpl.
func (s *hlsSource) run(ctx context.Context) error {
	var stream *stream
	var videoTrackID int
	var audioTrackID int
	var videoEnc *rtph264.Encoder
	var audioEnc *rtpaac.Encoder

	defer func() {
		if stream != nil {
			s.parent.onSourceStaticImplSetNotReady(pathSourceStaticSetNotReadyReq{})
		}
	}()

	onTracks := func(videoTrack *gortsplib.TrackH264, audioTrack *gortsplib.TrackAAC) error {
		var tracks gortsplib.Tracks

		if videoTrack != nil {
			videoTrackID = len(tracks)
			videoEnc = &rtph264.Encoder{PayloadType: 96}
			videoEnc.Init()
			tracks = append(tracks, videoTrack)
		}

		if audioTrack != nil {
			audioTrackID = len(tracks)
			audioEnc = &rtpaac.Encoder{
				PayloadType:      96,
				SampleRate:       audioTrack.ClockRate(),
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			}
			audioEnc.Init()
			tracks = append(tracks, audioTrack)
		}

		res := s.parent.onSourceStaticImplSetReady(pathSourceStaticSetReadyReq{tracks: tracks})
		if res.err != nil {
			return res.err
		}

		s.Log(logger.Info, "ready")
		stream = res.stream

		return nil
	}

	onVideoData := func(pts time.Duration, nalus [][]byte) {
		if stream == nil {
			return
		}

		pkts, err := videoEnc.Encode(nalus, pts)
		if err != nil {
			return
		}

		lastPkt := len(pkts) - 1
		for i, pkt := range pkts {
			if i != lastPkt {
				stream.writeData(&data{
					trackID:      videoTrackID,
					rtp:          pkt,
					ptsEqualsDTS: false,
				})
			} else {
				stream.writeData(&data{
					trackID:      videoTrackID,
					rtp:          pkt,
					ptsEqualsDTS: h264.IDRPresent(nalus),
					h264NALUs:    nalus,
					h264PTS:      pts,
				})
			}
		}
	}

	onAudioData := func(pts time.Duration, aus [][]byte) {
		if stream == nil {
			return
		}

		pkts, err := audioEnc.Encode(aus, pts)
		if err != nil {
			return
		}

		for _, pkt := range pkts {
			stream.writeData(&data{
				trackID:      audioTrackID,
				rtp:          pkt,
				ptsEqualsDTS: true,
			})
		}
	}

	c, err := hls.NewClient(
		s.ur,
		s.fingerprint,
		onTracks,
		onVideoData,
		onAudioData,
		s,
	)
	if err != nil {
		return err
	}

	select {
	case err := <-c.Wait():
		return err

	case <-ctx.Done():
		c.Close()
		<-c.Wait()
		return nil
	}
}

// onSourceAPIDescribe implements sourceStaticImpl.
func (*hlsSource) onSourceAPIDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"hlsSource"}
}
