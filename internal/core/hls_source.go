package core

import (
	"context"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"

	"github.com/aler9/rtsp-simple-server/internal/hls"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type hlsSourceParent interface {
	log(logger.Level, string, ...interface{})
	sourceStaticImplSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	sourceStaticImplSetNotReady(req pathSourceStaticSetNotReadyReq)
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

	defer func() {
		if stream != nil {
			s.parent.sourceStaticImplSetNotReady(pathSourceStaticSetNotReadyReq{})
		}
	}()

	onTracks := func(videoTrack *gortsplib.TrackH264, audioTrack *gortsplib.TrackMPEG4Audio) error {
		var tracks gortsplib.Tracks

		if videoTrack != nil {
			videoTrackID = len(tracks)
			tracks = append(tracks, videoTrack)
		}

		if audioTrack != nil {
			audioTrackID = len(tracks)
			tracks = append(tracks, audioTrack)
		}

		res := s.parent.sourceStaticImplSetReady(pathSourceStaticSetReadyReq{
			tracks:             tracks,
			generateRTPPackets: true,
		})
		if res.err != nil {
			return res.err
		}

		s.Log(logger.Info, "ready: %s", sourceTrackInfo(tracks))
		stream = res.stream

		return nil
	}

	onVideoData := func(pts time.Duration, nalus [][]byte) {
		if stream == nil {
			return
		}

		stream.writeData(&data{
			trackID:      videoTrackID,
			ptsEqualsDTS: h264.IDRPresent(nalus),
			pts:          pts,
			h264NALUs:    nalus,
		})
	}

	onAudioData := func(pts time.Duration, au []byte) {
		if stream == nil {
			return
		}

		stream.writeData(&data{
			trackID:      audioTrackID,
			ptsEqualsDTS: true,
			pts:          pts,
			mpeg4AudioAU: au,
		})
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

// apiSourceDescribe implements sourceStaticImpl.
func (*hlsSource) apiSourceDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"hlsSource"}
}
